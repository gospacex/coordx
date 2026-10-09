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

// ErrRegisterUnsupported Apollo 不提供服务注册能力。
//
// Apollo 是**配置中心**，其开放 API 只有配置发布/拉取，没有服务实例注册与
// 健康检查这组原语。本驱动保留只是为了契约完整（配置里 driver: apollo 不会
// 报"驱动未注册"），但 Register/Deregister 明确失败，不再假装成功。
//
// 修复前返回裸 error("apollo: not implemented in stub")，调用方无法用
// errors.Is 判定、也无法据此降级；现在同时 wrap coordx.ErrUnsupported，
// 调用方可用 errors.Is(err, coordx.ErrUnsupported) 统一处理。
var ErrRegisterUnsupported = fmt.Errorf("coordx/apollo: apollo is a config center with no service-registry API: %w", coordx.ErrUnsupported)

type registerImpl struct {
	cfg    *coordx.Config
	logger logx.Logger
	tracer *traceflag.Tracer
}

// NewRegister 创建 Apollo 注册中心
func NewRegister(cfg *coordx.Config) (*registerImpl, error) {
	// 下面 tracer 那行已考虑 cfg == nil，这里却直接解引用，自相矛盾。
	addr := ""
	if cfg != nil {
		addr = cfg.Addr
	}
	if addr == "" {
		addr = "http://localhost:8070"
	}
	r := &registerImpl{
		cfg:    cfg,
		logger: logx.GetGlobal(),
		tracer: traceflag.NewTracer(cfg != nil && cfg.EnableTrace, "github.com/gospacex/coordx/apollo", "coordx"),
	}
	// 用 Warn 而非 Info：此前打 Info 看着像"初始化成功、可以注册了"，
	// 实际 Register/Deregister 必然失败。地址也只是记下来，本驱动不发请求。
	r.logger.WarnContext(context.Background(), "coordx/apollo initialized but unsupported: apollo has no service-registry API",
		logx.String("addr", addr))
	return r, nil
}

func (r *registerImpl) Register(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	ctx, span := r.tracer.Start(ctx, "coord.register")
	defer span.End()
	// ⚠ nil 判断必须排在 inst.ID 解引用之前（与 nacos 驱动同一纪律）：
	// 否则 SetAttributes 里的 inst.ID 会先 panic。
	if inst == nil {
		err := fmt.Errorf("coordx/apollo: register %s: nil instance", service)
		span.RecordError(err)
		return err
	}
	span.SetAttributes(
		attribute.String("coord.driver", "apollo"),
		attribute.String("coord.service", service),
		attribute.String("coord.instance_id", inst.ID),
	)
	err := fmt.Errorf("coordx/apollo: register %s/%s: %w", service, inst.ID, ErrRegisterUnsupported)
	span.RecordError(err)
	r.logger.WarnContext(ctx, "coordx/apollo register unsupported", logx.String("service", service))
	return err
}
func (r *registerImpl) Deregister(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	ctx, span := r.tracer.Start(ctx, "coord.deregister")
	defer span.End()
	if inst == nil {
		err := fmt.Errorf("coordx/apollo: deregister %s: nil instance", service)
		span.RecordError(err)
		return err
	}
	span.SetAttributes(
		attribute.String("coord.driver", "apollo"),
		attribute.String("coord.service", service),
		attribute.String("coord.instance_id", inst.ID),
	)
	err := fmt.Errorf("coordx/apollo: deregister %s/%s: %w", service, inst.ID, ErrRegisterUnsupported)
	span.RecordError(err)
	r.logger.WarnContext(ctx, "coordx/apollo deregister unsupported", logx.String("service", service))
	return err
}
func (r *registerImpl) HealthCheck() error { return nil }
func (r *registerImpl) Close() error       { return nil }

func init() {
	coordx.RegisterCreator("apollo", func(cfg *coordx.Config) (coordx.Register, error) {
		return NewRegister(cfg)
	})
}
