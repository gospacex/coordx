// coordx/apollo: Apollo 配置中心驱动（用作注册中心）
package apollo

import (
	"context"
	"fmt"

	"github.com/gospacex/coordx"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
	"github.com/gospacex/traceflag"
	"go.opentelemetry.io/otel/attribute"
)

type registerImpl struct {
	cfg    *coordx.Config
	logger logx.Logger
	tracer *traceflag.Tracer
}

// NewRegister 创建 Apollo 注册中心
func NewRegister(cfg *coordx.Config) (*registerImpl, error) {
	addr := cfg.Addr
	if addr == "" {
		addr = "http://localhost:8070"
	}
	r := &registerImpl{
		cfg:    cfg,
		logger: logx.GetGlobal(),
		tracer: traceflag.NewTracer(cfg != nil && cfg.EnableTrace, "github.com/gospacex/coordx/apollo", "coordx"),
	}
	r.logger.InfoContext(context.Background(), "coordx/apollo initialized", logx.String("addr", addr))
	return r, nil
}

func (r *registerImpl) Register(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	ctx, span := r.tracer.Start(ctx, "coord.register")
	defer span.End()
	span.SetAttributes(
		attribute.String("coord.driver", "apollo"),
		attribute.String("coord.service", service),
		attribute.String("coord.instance_id", inst.ID),
	)
	r.logger.InfoContext(ctx, "coordx/apollo register stub", logx.String("service", service))
	err := fmt.Errorf("apollo: not implemented in stub")
	span.RecordError(err)
	return err
}
func (r *registerImpl) Deregister(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	ctx, span := r.tracer.Start(ctx, "coord.deregister")
	defer span.End()
	span.SetAttributes(
		attribute.String("coord.driver", "apollo"),
		attribute.String("coord.service", service),
		attribute.String("coord.instance_id", inst.ID),
	)
	r.logger.InfoContext(ctx, "coordx/apollo deregister stub", logx.String("service", service))
	err := fmt.Errorf("apollo: not implemented in stub")
	span.RecordError(err)
	return err
}
func (r *registerImpl) HealthCheck() error { return nil }
func (r *registerImpl) Close() error       { return nil }

func init() {
	coordx.RegisterCreator("apollo", func(cfg *coordx.Config) (coordx.Register, error) {
		return NewRegister(cfg)
	})
}
