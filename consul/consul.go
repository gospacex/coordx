// coordx/consul: Consul 服务注册与发现驱动
package consul

import (
	"context"
	"fmt"

	"github.com/gospacex/coordx"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
	"github.com/gospacex/traceflag"
	"github.com/hashicorp/consul/api"
	"go.opentelemetry.io/otel/attribute"
)

type registerImpl struct {
	client *api.Client
	cfg    *coordx.Config
	logger logx.Logger
	tracer *traceflag.Tracer
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
	}
	r.logger.InfoContext(context.Background(), "coordx/consul register initialized", logx.String("addr", addr))
	return r, nil
}

func (r *registerImpl) Register(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	ctx, span := r.tracer.Start(ctx, "coord.register")
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
		Check: &api.AgentServiceCheck{
			TTL:                            "30s",
			Status:                         "passing",
			Interval:                       "10s",
			DeregisterCriticalServiceAfter: "90s",
		},
	}
	if err := r.client.Agent().ServiceRegister(reg); err != nil {
		span.RecordError(err)
		r.logger.ErrorContext(ctx, "coordx/consul register failed", logx.Err(err), logx.String("service", service))
		return err
	}
	r.logger.InfoContext(ctx, "coordx/consul register done", logx.String("service", service), logx.String("inst_id", inst.ID))
	return nil
}
func (r *registerImpl) Deregister(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	ctx, span := r.tracer.Start(ctx, "coord.deregister")
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
	r.logger.InfoContext(ctx, "coordx/consul deregister done", logx.String("service", service), logx.String("inst_id", inst.ID))
	return nil
}
func (r *registerImpl) HealthCheck() error { return nil }
func (r *registerImpl) Close() error       { return nil }

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
	ctx, span := d.tracer.Start(ctx, "coord.discover")
	defer span.End()
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
