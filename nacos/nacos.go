// coordx/nacos: Nacos 服务注册与发现驱动
package nacos

import (
	"context"
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
)

type registerImpl struct {
	client naming_client.INamingClient
	cfg    *coordx.Config
	logger logx.Logger
	tracer *traceflag.Tracer
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

// NewRegister 创建 Nacos 注册中心
func NewRegister(cfg *coordx.Config) (*registerImpl, error) {
	sc := parseServerConfig(cfg.Addr)
	client, err := clients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  &constant.ClientConfig{TimeoutMs: 5000},
			ServerConfigs: []constant.ServerConfig{sc},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("coordx/nacos: create naming client: %w", err)
	}
	r := &registerImpl{client: client, cfg: cfg, logger: logx.GetGlobal()}
	r.logger.InfoContext(context.Background(), "coordx/nacos register initialized", logx.String("addr", cfg.Addr))
	return r, nil
}

func (r *registerImpl) Register(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	ctx, span := r.tracer.Start(ctx, "coord.register")
	defer span.End()
	span.SetAttributes(
		attribute.String("coord.driver", "nacos"),
		attribute.String("coord.service", service),
		attribute.String("coord.instance_id", inst.ID),
	)
	// stub: nacos naming clientRegister would go here
	r.logger.InfoContext(ctx, "coordx/nacos register stub", logx.String("service", service), logx.String("inst_id", inst.ID))
	err := fmt.Errorf("nacos: stub")
	span.RecordError(err)
	return err
}
func (r *registerImpl) Deregister(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	ctx, span := r.tracer.Start(ctx, "coord.deregister")
	defer span.End()
	span.SetAttributes(
		attribute.String("coord.driver", "nacos"),
		attribute.String("coord.service", service),
		attribute.String("coord.instance_id", inst.ID),
	)
	r.logger.InfoContext(ctx, "coordx/nacos deregister stub", logx.String("service", service))
	err := fmt.Errorf("nacos: stub")
	span.RecordError(err)
	return err
}
func (r *registerImpl) HealthCheck() error { return nil }
func (r *registerImpl) Close() error       { return nil }

// NewDiscover 创建 Nacos 发现器
func NewDiscover(cfg *coordx.Config) (*discoverImpl, error) {
	sc := parseServerConfig(cfg.Addr)
	client, err := clients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  &constant.ClientConfig{TimeoutMs: 5000},
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
	ctx, span := d.tracer.Start(ctx, "coord.discover")
	defer span.End()
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
	ctx, span := d.tracer.Start(ctx, "coord.watch")
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
func NewLease(cfg *coordx.Config) (*leaseImpl, error) {
	sc := constant.ServerConfig{ContextPath: "/nacos", Port: 8848, Scheme: "http"}
	client, err := clients.NewConfigClient(
		vo.NacosClientParam{
			ClientConfig:  &constant.ClientConfig{TimeoutMs: 5000},
			ServerConfigs: []constant.ServerConfig{sc},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("coordx/nacos: create config client: %w", err)
	}
	_ = client
	l := &leaseImpl{cfg: cfg, logger: logx.GetGlobal()}
	l.logger.InfoContext(context.Background(), "coordx/nacos lease initialized", logx.String("addr", cfg.Addr))
	return l, nil
}
func (l *leaseImpl) Grant(ctx context.Context, ttl time.Duration) (string, error) {
	return "", fmt.Errorf("stub")
}
func (l *leaseImpl) Renew(ctx context.Context, sessionID string) error  { return nil }
func (l *leaseImpl) Revoke(ctx context.Context, sessionID string) error { return nil }
func (l *leaseImpl) HealthCheck() error                                 { return nil }
func (l *leaseImpl) Close() error                                       { return nil }

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
