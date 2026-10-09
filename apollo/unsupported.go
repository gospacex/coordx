// coordx/apollo: 补齐统一入口（coordx.ReadAndCreate）所需的能力占位实现
//
// 背景：coordx.ReadAndCreate 按"三件套"契约依次创建 Register / Discover /
// Lease，任何一个 creator 未注册就整体返回
// `coordx: lease driver "apollo" not registered` —— 于是 apollo 这个驱动
// **根本无法通过统一入口使用**（consul / etcd / k8s_configmap 同样如此，
// 五个驱动里只有 nacos 注册齐了三项）。
//
// 修法选择：不改 ReadAndCreate 的失败语义（注册了才真实创建，这条不变），
// 而是把驱动**不具备**的能力补注册为"显式不可用"实现——
//   - 调用方仍能走统一入口，不会被"驱动未注册"卡住；
//   - 真正调用缺失能力时拿到 wrap 了 coordx.ErrUnsupported 的具名错误，
//     既能 errors.Is(err, coordx.ErrUnsupported) 统一降级，也能精确判定；
//   - 绝不像修复前的 nacos Lease.Renew 那样返回 nil 假装成功。
package apollo

import (
	"context"
	"fmt"
	"time"

	"github.com/gospacex/coordx"
	"github.com/gospacex/logx"
	"github.com/gospacex/traceflag"
)

// ErrDiscoverUnsupported Apollo 不提供服务发现能力：它是配置中心，
// 既没有实例注册表也没有健康检查原语，Discovery 无从实现。
var ErrDiscoverUnsupported = fmt.Errorf("coordx/apollo: apollo is a config center with no service-discovery API: %w", coordx.ErrUnsupported)

// ErrLeaseUnsupported Apollo 不提供分布式租约能力。
var ErrLeaseUnsupported = fmt.Errorf("coordx/apollo: apollo has no distributed-lease API: %w", coordx.ErrUnsupported)

// unsupportedDiscover 占位发现器：构造成功，调用必失败且可判定。
type unsupportedDiscover struct {
	logger logx.Logger
	tracer *traceflag.Tracer
}

// NewDiscover 创建占位发现器（统一入口需要；实际调用返回 ErrDiscoverUnsupported）
func NewDiscover(cfg *coordx.Config) (*unsupportedDiscover, error) {
	return &unsupportedDiscover{
		logger: logx.GetGlobal(),
		tracer: traceflag.NewTracer(cfg != nil && cfg.EnableTrace, "github.com/gospacex/coordx/apollo", "coordx"),
	}, nil
}

func (d *unsupportedDiscover) Discovery(ctx context.Context, service string) ([]*coordx.ServiceInstance, error) {
	ctx, span := d.tracer.Start(ctx, "coord.discovery")
	defer span.End()
	err := fmt.Errorf("coordx/apollo: discovery %s: %w", service, ErrDiscoverUnsupported)
	span.RecordError(err)
	d.logger.WarnContext(ctx, "coordx/apollo discovery unsupported", logx.String("service", service))
	return nil, err
}

func (d *unsupportedDiscover) Watch(ctx context.Context, service string, callback func([]*coordx.ServiceInstance)) error {
	ctx, span := d.tracer.Start(ctx, "coord.watch")
	defer span.End()
	err := fmt.Errorf("coordx/apollo: watch %s: %w", service, ErrDiscoverUnsupported)
	span.RecordError(err)
	d.logger.WarnContext(ctx, "coordx/apollo watch unsupported", logx.String("service", service))
	return err
}

// HealthCheck 能力不可用，返回可判定错误而不是 nil —— 返回 nil 会让调用方
// 误以为"这个发现器是健康的、可以用"。
func (d *unsupportedDiscover) HealthCheck() error { return ErrDiscoverUnsupported }
func (d *unsupportedDiscover) Close() error       { return nil }

// unsupportedLease 占位租约：同上，构造成功、调用必失败。
type unsupportedLease struct {
	logger logx.Logger
	tracer *traceflag.Tracer
}

// NewLease 创建占位租约（统一入口需要；实际调用返回 ErrLeaseUnsupported）
func NewLease(cfg *coordx.Config) (*unsupportedLease, error) {
	return &unsupportedLease{
		logger: logx.GetGlobal(),
		tracer: traceflag.NewTracer(cfg != nil && cfg.EnableTrace, "github.com/gospacex/coordx/apollo", "coordx"),
	}, nil
}

func (l *unsupportedLease) Grant(ctx context.Context, ttl time.Duration) (string, error) {
	ctx, span := l.tracer.Start(ctx, "coord.lease.grant")
	defer span.End()
	err := fmt.Errorf("coordx/apollo: lease grant: %w", ErrLeaseUnsupported)
	span.RecordError(err)
	l.logger.WarnContext(ctx, "coordx/apollo lease grant unsupported")
	return "", err
}
func (l *unsupportedLease) Renew(ctx context.Context, sessionID string) error {
	ctx, span := l.tracer.Start(ctx, "coord.lease.renew")
	defer span.End()
	err := fmt.Errorf("coordx/apollo: lease renew %s: %w", sessionID, ErrLeaseUnsupported)
	span.RecordError(err)
	return err
}
func (l *unsupportedLease) Revoke(ctx context.Context, sessionID string) error {
	ctx, span := l.tracer.Start(ctx, "coord.lease.revoke")
	defer span.End()
	err := fmt.Errorf("coordx/apollo: lease revoke %s: %w", sessionID, ErrLeaseUnsupported)
	span.RecordError(err)
	return err
}
func (l *unsupportedLease) HealthCheck() error { return ErrLeaseUnsupported }
func (l *unsupportedLease) Close() error       { return nil }
