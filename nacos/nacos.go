// coordx/nacos: Nacos 服务注册与发现驱动
package nacos

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gospacex/coordx"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
	"github.com/gospacex/traceflag"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type registerImpl struct {
	client naming_client.INamingClient
	cfg    *coordx.Config
	logger logx.Logger
	tracer *traceflag.Tracer
}

// ErrLeaseUnsupported Nacos 不支持 Lease 语义时返回的哨兵错误，调用方可用
// errors.Is 判定并降级（例如改用 ephemeral 心跳自带的存活语义）。
//
// 原因：Nacos 的临时实例（ephemeral）由**客户端心跳**维持存活，服务端没有
// 「申请租约 / 续租 / 释放」这组原语，与 etcd 的 Lease 模型不同。此前三个方法
// 分别返回裸 error("stub")、nil、nil——Renew/Revoke 返回 nil 会让调用方误以为
// 续约成功，比报错更危险，现统一显式报错。
var ErrLeaseUnsupported = fmt.Errorf("coordx/nacos: lease not supported by nacos (ephemeral heartbeat model): %w", coordx.ErrUnsupported)

// startIfParent 只在 ctx 内已有有效父 span 时才开埋点 span；无父（服务启停、
// 后台 watch 等框架调度路径，调用方拿的是 context.Background()）返回 noop
// span。这些调用没有上游请求，开了 span 也只会成为无父孤儿根 trace，把
// Jaeger 列表淹成噪声（与 Discovery 的门控同一纪律）。noop 下下方
// SetAttributes / RecordError 语义不变。
func startIfParent(ctx context.Context, tracer *traceflag.Tracer, name string) (context.Context, trace.Span) {
	if trace.SpanContextFromContext(ctx).IsValid() {
		return tracer.Start(ctx, name)
	}
	return ctx, trace.SpanFromContext(ctx)
}

type discoverImpl struct {
	client naming_client.INamingClient
	cfg    *coordx.Config
	logger logx.Logger
	tracer *traceflag.Tracer
}

type leaseImpl struct {
	cfg    *coordx.Config
	logger logx.Logger
}

// parseServerConfig 从 cfg.Addr 解析 nacos ServerConfig（支持 host:port 或 http://host:port）
func parseServerConfig(addr string) constant.ServerConfig {
	sc := constant.ServerConfig{ContextPath: "/nacos", Port: 8848, Scheme: "http"}
	if addr == "" {
		return sc
	}
	s := addr
	if i := strings.Index(s, "//"); i >= 0 {
		s = s[i+2:]
	}
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, ":"); i > 0 {
		if p, err := strconv.Atoi(s[i+1:]); err == nil {
			sc.IpAddr = s[:i]
			sc.Port = uint64(p)
			return sc
		}
	}
	sc.IpAddr = s
	return sc
}

// newClientConfig 构造 Nacos 客户端配置。
//
// cfg.Cluster 在本驱动承载 Nacos 的 namespaceId（见 config.go 的
// Cluster: s.Namespace 映射）。此前两处 New* 都只写 TimeoutMs，namespace 配了
// 也不生效，属僵尸配置字段。
func newClientConfig(cfg *coordx.Config) *constant.ClientConfig {
	cc := &constant.ClientConfig{TimeoutMs: 5000}
	if cfg != nil {
		cc.NamespaceId = cfg.Cluster
	}
	return cc
}

// NewRegister 创建 Nacos 注册中心
func NewRegister(cfg *coordx.Config) (*registerImpl, error) {
	sc := parseServerConfig(cfg.Addr)
	client, err := clients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  newClientConfig(cfg),
			ServerConfigs: []constant.ServerConfig{sc},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("coordx/nacos: create naming client: %w", err)
	}
	r := &registerImpl{
		client: client,
		cfg:    cfg,
		logger: logx.GetGlobal(),
		// ⚠ 此前此处漏了 tracer 初始化：Register/Deregister 里的
		// r.tracer.Start 会在 nil receiver 上解引用 t.local 直接 panic
		// （traceflag.Tracer.Start → Enabled）。由于 startIfParent 只在有父
		// span 时才调用，启动期用 context.Background() 恰好绕开了 panic，
		// 但代价是 Register 永远产不出 span，且一旦调用方在请求上下文里
		// 注册就会崩。
		tracer: traceflag.NewTracer(cfg != nil && cfg.EnableTrace, "github.com/gospacex/coordx/nacos", "coordx"),
	}
	r.logger.InfoContext(context.Background(), "coordx/nacos register initialized", logx.String("addr", cfg.Addr))
	return r, nil
}

func (r *registerImpl) Register(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	// ⚠ nil 判断必须排在任何 inst.ID 解引用之前：否则 SetAttributes 里的
	// inst.ID 会先 panic，nil 分支形同虚设。
	ctx, span := startIfParent(ctx, r.tracer, "coord.register")
	defer span.End()
	if inst == nil {
		err := errors.New("coordx/nacos: register: nil instance")
		span.RecordError(err)
		return err
	}
	span.SetAttributes(
		attribute.String("coord.driver", "nacos"),
		attribute.String("coord.service", service),
		attribute.String("coord.instance_id", inst.ID),
	)
	// Weight 必须 > 0（Nacos 校验），无权重概念故固定 1。
	// Healthy 固定 true：ServiceInstance.Healthy 零值为 false，若直接透传会让
	// 未显式设 Healthy 的实例注册成不健康、对消费者不可见（与 consul 驱动
	// 的 Status: "passing" 同一取舍）。实例存活改由 Ephemeral 心跳判定。
	// Ephemeral=true：临时实例，进程退出后由服务端自动摘除，不留僵尸节点。
	ok, err := r.client.RegisterInstance(vo.RegisterInstanceParam{
		Ip:          inst.Addr,
		Port:        uint64(inst.Port),
		ServiceName: service,
		GroupName:   constant.DEFAULT_GROUP,
		Metadata:    inst.Metadata,
		Weight:      1,
		Enable:      true,
		Healthy:     true,
		Ephemeral:   true,
	})
	if err != nil {
		span.RecordError(err)
		r.logger.ErrorContext(ctx, "coordx/nacos register failed", logx.Err(err), logx.String("service", service), logx.String("inst_id", inst.ID))
		return fmt.Errorf("coordx/nacos: register %s/%s: %w", service, inst.ID, err)
	}
	if !ok {
		// RegisterInstance 返回 (false, nil) 表示服务端拒绝但未给原因，
		// 不能当成功吞掉——否则调用方以为注册成功，实际服务永不可见。
		err := fmt.Errorf("coordx/nacos: register %s/%s: nacos returned false without error", service, inst.ID)
		span.RecordError(err)
		r.logger.ErrorContext(ctx, "coordx/nacos register rejected", logx.String("service", service), logx.String("inst_id", inst.ID))
		return err
	}
	r.logger.InfoContext(ctx, "coordx/nacos register done", logx.String("service", service), logx.String("inst_id", inst.ID))
	return nil
}
func (r *registerImpl) Deregister(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	ctx, span := startIfParent(ctx, r.tracer, "coord.deregister")
	defer span.End()
	// 同 Register：nil 判断必须排在 inst.ID 解引用之前。
	if inst == nil {
		err := errors.New("coordx/nacos: deregister: nil instance")
		span.RecordError(err)
		return err
	}
	span.SetAttributes(
		attribute.String("coord.driver", "nacos"),
		attribute.String("coord.service", service),
		attribute.String("coord.instance_id", inst.ID),
	)
	// Ephemeral 必须与注册时一致，否则服务端按持久化实例去找、摘不掉。
	ok, err := r.client.DeregisterInstance(vo.DeregisterInstanceParam{
		Ip:          inst.Addr,
		Port:        uint64(inst.Port),
		ServiceName: service,
		GroupName:   constant.DEFAULT_GROUP,
		Ephemeral:   true,
	})
	if err != nil {
		span.RecordError(err)
		r.logger.ErrorContext(ctx, "coordx/nacos deregister failed", logx.Err(err), logx.String("service", service), logx.String("inst_id", inst.ID))
		return fmt.Errorf("coordx/nacos: deregister %s/%s: %w", service, inst.ID, err)
	}
	if !ok {
		err := fmt.Errorf("coordx/nacos: deregister %s/%s: nacos returned false without error", service, inst.ID)
		span.RecordError(err)
		r.logger.ErrorContext(ctx, "coordx/nacos deregister rejected", logx.String("service", service), logx.String("inst_id", inst.ID))
		return err
	}
	r.logger.InfoContext(ctx, "coordx/nacos deregister done", logx.String("service", service), logx.String("inst_id", inst.ID))
	return nil
}
func (r *registerImpl) HealthCheck() error { return nil }
func (r *registerImpl) Close() error       { return nil }

// NewDiscover 创建 Nacos 发现器
func NewDiscover(cfg *coordx.Config) (*discoverImpl, error) {
	sc := parseServerConfig(cfg.Addr)
	client, err := clients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  newClientConfig(cfg),
			ServerConfigs: []constant.ServerConfig{sc},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("coordx/nacos: create naming client: %w", err)
	}
	d := &discoverImpl{
		client: client,
		cfg:    cfg,
		logger: logx.GetGlobal(),
		tracer: traceflag.NewTracer(cfg != nil && cfg.EnableTrace, "github.com/gospacex/coordx/nacos", "coordx"),
	}
	d.logger.InfoContext(context.Background(), "coordx/nacos discover initialized", logx.String("addr", cfg.Addr))
	return d, nil
}

func (d *discoverImpl) Discovery(ctx context.Context, service string) ([]*coordx.ServiceInstance, error) {
	// 后台轮询（如 kitex 周期 resolve）传入的 ctx 不带父 span：若照样 Start，
	// 每次轮询都另起一根孤儿单 span trace，Jaeger 列表被噪声淹没，真实请求
	// 的 trace 反而翻不到。故只在请求内（ctx 已有有效父 span）才埋点；
	// 无父时用 noop span 占位，下方 SetAttributes / RecordError 语义不变。
	span := trace.SpanFromContext(ctx)
	if trace.SpanContextFromContext(ctx).IsValid() {
		ctx, span = d.tracer.Start(ctx, "coord.discover")
		defer span.End()
	}
	span.SetAttributes(
		attribute.String("coord.driver", "nacos"),
		attribute.String("coord.service", service),
	)
	list, err := d.client.SelectAllInstances(vo.SelectAllInstancesParam{
		ServiceName: service,
	})
	if err != nil {
		span.RecordError(err)
		d.logger.ErrorContext(ctx, "coordx/nacos discovery failed", logx.Err(err), logx.String("service", service))
		return nil, err
	}
	var result []*coordx.ServiceInstance
	for _, inst := range list {
		result = append(result, &coordx.ServiceInstance{
			ID:      inst.InstanceId,
			Addr:    inst.Ip,
			Port:    int(inst.Port),
			Healthy: inst.Healthy,
		})
	}
	span.SetAttributes(attribute.Int("coord.instance_count", len(result)))
	d.logger.InfoContext(ctx, "coordx/nacos discovery done", logx.String("service", service), logx.Int("instances", len(result)))
	return result, nil
}
func (d *discoverImpl) Watch(ctx context.Context, service string, callback func([]*coordx.ServiceInstance)) error {
	ctx, span := startIfParent(ctx, d.tracer, "coord.watch")
	defer span.End()
	span.SetAttributes(
		attribute.String("coord.driver", "nacos"),
		attribute.String("coord.service", service),
	)
	d.logger.InfoContext(ctx, "coordx/nacos watch started", logx.String("service", service))
	err := d.client.Subscribe(&vo.SubscribeParam{
		ServiceName: service,
		GroupName:   constant.DEFAULT_GROUP,
		SubscribeCallback: func(services []model.Instance, err error) {
			if err != nil || callback == nil {
				return
			}
			insts := make([]*coordx.ServiceInstance, 0, len(services))
			for _, inst := range services {
				insts = append(insts, &coordx.ServiceInstance{
					ID:      inst.InstanceId,
					Addr:    inst.Ip,
					Port:    int(inst.Port),
					Healthy: inst.Healthy,
				})
			}
			callback(insts)
		},
	})
	if err != nil {
		span.RecordError(err)
	}
	return err
}
func (d *discoverImpl) HealthCheck() error { return nil }
func (d *discoverImpl) Close() error       { return nil }

// NewLease 创建 Nacos Lease
//
// Nacos 没有租约原语，Grant/Renew/Revoke 一律返回 ErrLeaseUnsupported。
// 此前这里建了一个 config client 却 `_ = client` 丢弃，且 ServerConfig 硬编码
// 8848、完全忽略 cfg.Addr——既浪费连接又误导（看着像能连实际没用）。现已删除，
// 构造只保留配置与日志，让"不支持"在编译期就无法伪装成支持。
func NewLease(cfg *coordx.Config) (*leaseImpl, error) {
	l := &leaseImpl{cfg: cfg, logger: logx.GetGlobal()}
	addr := ""
	if cfg != nil {
		addr = cfg.Addr
	}
	l.logger.WarnContext(context.Background(), "coordx/nacos lease is not supported: nacos uses ephemeral heartbeat, no lease primitive",
		logx.String("addr", addr))
	return l, nil
}

func (l *leaseImpl) Grant(ctx context.Context, ttl time.Duration) (string, error) {
	err := fmt.Errorf("coordx/nacos: grant(ttl=%s): %w", ttl, ErrLeaseUnsupported)
	trace.SpanFromContext(ctx).RecordError(err)
	l.logger.WarnContext(ctx, "coordx/nacos lease grant unsupported", logx.String("ttl", ttl.String()))
	return "", err
}

// Renew 此前返回 nil，会让调用方误以为续约成功（实际什么都没做），
// 比显式报错危险得多。
func (l *leaseImpl) Renew(ctx context.Context, sessionID string) error {
	err := fmt.Errorf("coordx/nacos: renew(%s): %w", sessionID, ErrLeaseUnsupported)
	trace.SpanFromContext(ctx).RecordError(err)
	return err
}

func (l *leaseImpl) Revoke(ctx context.Context, sessionID string) error {
	err := fmt.Errorf("coordx/nacos: revoke(%s): %w", sessionID, ErrLeaseUnsupported)
	trace.SpanFromContext(ctx).RecordError(err)
	return err
}
func (l *leaseImpl) HealthCheck() error { return nil }
func (l *leaseImpl) Close() error       { return nil }

func init() {
	coordx.RegisterCreator("nacos", func(cfg *coordx.Config) (coordx.Register, error) {
		return NewRegister(cfg)
	})
	coordx.RegisterDiscoverCreator("nacos", func(cfg *coordx.Config) (coordx.Discover, error) {
		return NewDiscover(cfg)
	})
	coordx.RegisterLeaseCreator("nacos", func(cfg *coordx.Config) (coordx.Lease, error) {
		return NewLease(cfg)
	})
}
