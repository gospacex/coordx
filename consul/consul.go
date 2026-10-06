// coordx/consul: Consul 服务注册与发现驱动
package consul

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gospacex/coordx"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
	"github.com/gospacex/traceflag"
	"github.com/hashicorp/consul/api"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ttlBeatInterval TTL 心跳周期：须小于 Check.TTL（30s），取 1/3 留足三倍余量。
const ttlBeatInterval = 10 * time.Second

type registerImpl struct {
	client *api.Client
	cfg    *coordx.Config
	logger logx.Logger
	tracer *traceflag.Tracer

	mu    sync.Mutex
	beats map[string]chan struct{} // instanceID -> stop channel
}

// startIfParent 只在 ctx 内已有有效父 span 时才开埋点 span；无父（服务启停
// 等框架调度路径，调用方拿的是 context.Background()）返回 noop span。这些
// 调用没有上游请求，开了 span 也只会成为无父孤儿根 trace（与 Discovery 的
// 门控同一纪律）。noop 下下方 SetAttributes / RecordError 语义不变。
func startIfParent(ctx context.Context, tracer *traceflag.Tracer, name string) (context.Context, trace.Span) {
	if trace.SpanContextFromContext(ctx).IsValid() {
		return tracer.Start(ctx, name)
	}
	return ctx, trace.SpanFromContext(ctx)
}

type discoverImpl struct {
	client *api.Client
	cfg    *coordx.Config
	logger logx.Logger
	tracer *traceflag.Tracer
}

// NewRegister 创建 Consul 注册中心
func NewRegister(cfg *coordx.Config) (*registerImpl, error) {
	addr := "http://localhost:8500"
	if cfg.Addr != "" {
		addr = cfg.Addr
	}
	c, err := api.NewClient(&api.Config{Address: addr})
	if err != nil {
		return nil, fmt.Errorf("coordx/consul: create client: %w", err)
	}
	r := &registerImpl{
		client: c,
		cfg:    cfg,
		logger: logx.GetGlobal(),
		tracer: traceflag.NewTracer(cfg != nil && cfg.EnableTrace, "github.com/gospacex/coordx/consul", "coordx"),
		beats:  make(map[string]chan struct{}),
	}
	r.logger.InfoContext(context.Background(), "coordx/consul register initialized", logx.String("addr", addr))
	return r, nil
}

func (r *registerImpl) Register(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	ctx, span := startIfParent(ctx, r.tracer, "coord.register")
	defer span.End()
	span.SetAttributes(
		attribute.String("coord.driver", "consul"),
		attribute.String("coord.service", service),
		attribute.String("coord.instance_id", inst.ID),
	)
	reg := &api.AgentServiceRegistration{
		ID:      inst.ID,
		Name:    service,
		Address: inst.Addr,
		Port:    inst.Port,
		// TTL 检查与 Interval 互斥（Consul 校验：Interval and TTL cannot both be
		// specified），只保留 TTL，由 startBeat 周期性 UpdateTTL 维持 passing。
		Check: &api.AgentServiceCheck{
			TTL:                            "30s",
			Status:                         "passing",
			DeregisterCriticalServiceAfter: "90s",
		},
	}
	if err := r.client.Agent().ServiceRegister(reg); err != nil {
		span.RecordError(err)
		r.logger.ErrorContext(ctx, "coordx/consul register failed", logx.Err(err), logx.String("service", service))
		return err
	}
	r.startBeat(inst.ID)
	r.logger.InfoContext(ctx, "coordx/consul register done", logx.String("service", service), logx.String("inst_id", inst.ID))
	return nil
}

// startBeat 启动实例 TTL 心跳；同 ID 重复注册先停旧心跳，防止双写。
func (r *registerImpl) startBeat(id string) {
	if id == "" {
		return
	}
	r.stopBeat(id)
	stop := make(chan struct{})
	r.mu.Lock()
	r.beats[id] = stop
	r.mu.Unlock()
	go func() {
		t := time.NewTicker(ttlBeatInterval)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if err := r.client.Agent().UpdateTTL("service:"+id, "", "passing"); err != nil {
					r.logger.WarnContext(context.Background(), "coordx/consul ttl heartbeat failed",
						logx.Err(err), logx.String("inst_id", id))
				}
			}
		}
	}()
}

// stopBeat 停止并移除实例 TTL 心跳（不存在时为 no-op）。
func (r *registerImpl) stopBeat(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if stop, ok := r.beats[id]; ok {
		close(stop)
		delete(r.beats, id)
	}
}
func (r *registerImpl) Deregister(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	ctx, span := startIfParent(ctx, r.tracer, "coord.deregister")
	defer span.End()
	span.SetAttributes(
		attribute.String("coord.driver", "consul"),
		attribute.String("coord.service", service),
		attribute.String("coord.instance_id", inst.ID),
	)
	if err := r.client.Agent().ServiceDeregister(inst.ID); err != nil {
		span.RecordError(err)
		r.logger.ErrorContext(ctx, "coordx/consul deregister failed", logx.Err(err), logx.String("service", service))
		return err
	}
	r.stopBeat(inst.ID)
	r.logger.InfoContext(ctx, "coordx/consul deregister done", logx.String("service", service), logx.String("inst_id", inst.ID))
	return nil
}
func (r *registerImpl) HealthCheck() error { return nil }
func (r *registerImpl) Close() error {
	r.mu.Lock()
	for id, stop := range r.beats {
		close(stop)
		delete(r.beats, id)
	}
	r.mu.Unlock()
	return nil
}

// NewDiscover 创建 Consul 发现器
func NewDiscover(cfg *coordx.Config) (*discoverImpl, error) {
	addr := "http://localhost:8500"
	if cfg.Addr != "" {
		addr = cfg.Addr
	}
	c, err := api.NewClient(&api.Config{Address: addr})
	if err != nil {
		return nil, fmt.Errorf("coordx/consul: create client: %w", err)
	}
	d := &discoverImpl{
		client: c,
		cfg:    cfg,
		logger: logx.GetGlobal(),
		tracer: traceflag.NewTracer(cfg != nil && cfg.EnableTrace, "github.com/gospacex/coordx/consul", "coordx"),
	}
	d.logger.InfoContext(context.Background(), "coordx/consul discover initialized", logx.String("addr", addr))
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
		attribute.String("coord.driver", "consul"),
		attribute.String("coord.service", service),
	)
	entries, _, err := d.client.Health().Service(service, "", true, nil)
	if err != nil {
		span.RecordError(err)
		d.logger.ErrorContext(ctx, "coordx/consul discovery failed", logx.Err(err), logx.String("service", service))
		return nil, err
	}
	var result []*coordx.ServiceInstance
	for _, e := range entries {
		healthy := false
		for _, c := range e.Checks {
			if c != nil && c.Status == "passing" {
				healthy = true
				break
			}
		}
		result = append(result, &coordx.ServiceInstance{
			ID:        e.Service.ID,
			ServiceID: e.Service.ID,
			Addr:      e.Service.Address,
			Port:      e.Service.Port,
			Healthy:   healthy,
		})
	}
	span.SetAttributes(attribute.Int("coord.instance_count", len(result)))
	d.logger.InfoContext(ctx, "coordx/consul discovery done", logx.String("service", service), logx.Int("instances", len(result)))
	return result, nil
}
func (d *discoverImpl) Watch(ctx context.Context, service string, callback func([]*coordx.ServiceInstance)) error {
	d.logger.InfoContext(ctx, "coordx/consul watch started", logx.String("service", service))
	return nil
}
func (d *discoverImpl) HealthCheck() error { return nil }
func (d *discoverImpl) Close() error       { return nil }

func init() {
	coordx.RegisterCreator("consul", func(cfg *coordx.Config) (coordx.Register, error) {
		return NewRegister(cfg)
	})
	coordx.RegisterDiscoverCreator("consul", func(cfg *coordx.Config) (coordx.Discover, error) {
		return NewDiscover(cfg)
	})
}
