// coordx/k8s_configmap: 补齐统一入口（coordx.ReadAndCreate）所需的能力占位实现
//
// 背景：coordx.ReadAndCreate 按"三件套"契约依次创建 Register / Discover /
// Lease，任一 creator 未注册就整体返回
// `coordx: lease driver "k8s_configmap" not registered` —— 本驱动此前只注册
// 了 Register，**无法通过统一入口使用**（五个驱动里只有 nacos 注册齐三项）。
//
// 修法：不改 ReadAndCreate 的失败语义，而是把未实现的能力注册为"显式不可用"
// 实现——统一入口可用，真调用时返回 wrap coordx.ErrUnsupported 的具名错误，
// 可 errors.Is 判定并降级，绝不返回 nil 假装成功。
//
// 注：K8s ConfigMap 理论上能承载简易的服务发现（List/Watch ConfigMap），
// 但本驱动未实现；与其静默返回空列表，不如明确报错，等真正实现时替换本文件。
package k8sconfigmap

import (
	"context"
	"fmt"
	"time"

	"github.com/gospacex/coordx"
	"github.com/gospacex/logx"
	"github.com/gospacex/traceflag"
)

// ErrDiscoverUnsupported ConfigMap 驱动尚未实现服务发现。
var ErrDiscoverUnsupported = fmt.Errorf("coordx/k8s_configmap: service discovery not implemented: %w", coordx.ErrUnsupported)

// ErrLeaseUnsupported K8s ConfigMap 不提供分布式租约能力
// （ConfigMap 无 TTL / 无租约原语，强行模拟只会得到不可靠的锁）。
var ErrLeaseUnsupported = fmt.Errorf("coordx/k8s_configmap: configmap has no lease primitive: %w", coordx.ErrUnsupported)

// unsupportedDiscover 占位发现器：构造成功，调用必失败且可判定。
type unsupportedDiscover struct {
	logger logx.Logger
	tracer *traceflag.Tracer
}

// NewDiscover 创建占位发现器（统一入口需要；实际调用返回 ErrDiscoverUnsupported）
func NewDiscover(cfg *coordx.Config) (*unsupportedDiscover, error) {
	return &unsupportedDiscover{
		logger: logx.GetGlobal(),
		tracer: traceflag.NewTracer(cfg != nil && cfg.EnableTrace, "github.com/gospacex/coordx/k8s_configmap", "coordx"),
	}, nil
}

func (d *unsupportedDiscover) Discovery(ctx context.Context, service string) ([]*coordx.ServiceInstance, error) {
	ctx, span := d.tracer.Start(ctx, "coord.discovery")
	defer span.End()
	err := fmt.Errorf("coordx/k8s_configmap: discovery %s: %w", service, ErrDiscoverUnsupported)
	span.RecordError(err)
	d.logger.WarnContext(ctx, "coordx/k8s_configmap discovery unsupported", logx.String("service", service))
	return nil, err
}

func (d *unsupportedDiscover) Watch(ctx context.Context, service string, callback func([]*coordx.ServiceInstance)) error {
	ctx, span := d.tracer.Start(ctx, "coord.watch")
	defer span.End()
	err := fmt.Errorf("coordx/k8s_configmap: watch %s: %w", service, ErrDiscoverUnsupported)
	span.RecordError(err)
	d.logger.WarnContext(ctx, "coordx/k8s_configmap watch unsupported", logx.String("service", service))
	return err
}

// HealthCheck 不可用能力返回可判定错误，不返回 nil（nil 会被当成"健康"）。
func (d *unsupportedDiscover) HealthCheck() error { return ErrDiscoverUnsupported }
func (d *unsupportedDiscover) Close() error       { return nil }

// unsupportedLease 占位租约：构造成功、调用必失败。
type unsupportedLease struct {
	logger logx.Logger
	tracer *traceflag.Tracer
}

// NewLease 创建占位租约（统一入口需要；实际调用返回 ErrLeaseUnsupported）
func NewLease(cfg *coordx.Config) (*unsupportedLease, error) {
	return &unsupportedLease{
		logger: logx.GetGlobal(),
		tracer: traceflag.NewTracer(cfg != nil && cfg.EnableTrace, "github.com/gospacex/coordx/k8s_configmap", "coordx"),
	}, nil
}

func (l *unsupportedLease) Grant(ctx context.Context, ttl time.Duration) (string, error) {
	ctx, span := l.tracer.Start(ctx, "coord.lease.grant")
	defer span.End()
	err := fmt.Errorf("coordx/k8s_configmap: lease grant: %w", ErrLeaseUnsupported)
	span.RecordError(err)
	l.logger.WarnContext(ctx, "coordx/k8s_configmap lease grant unsupported")
	return "", err
}
func (l *unsupportedLease) Renew(ctx context.Context, sessionID string) error {
	ctx, span := l.tracer.Start(ctx, "coord.lease.renew")
	defer span.End()
	return fmt.Errorf("coordx/k8s_configmap: lease renew %s: %w", sessionID, ErrLeaseUnsupported)
}
func (l *unsupportedLease) Revoke(ctx context.Context, sessionID string) error {
	ctx, span := l.tracer.Start(ctx, "coord.lease.revoke")
	defer span.End()
	return fmt.Errorf("coordx/k8s_configmap: lease revoke %s: %w", sessionID, ErrLeaseUnsupported)
}
func (l *unsupportedLease) HealthCheck() error { return ErrLeaseUnsupported }
func (l *unsupportedLease) Close() error       { return nil }
