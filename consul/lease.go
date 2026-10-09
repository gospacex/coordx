// coordx/consul: 分布式租约实现（Consul Session）
//
// 背景：coordx.ReadAndCreate 按"三件套"契约依次创建 Register / Discover /
// Lease，而本驱动此前只注册了前两个，导致统一入口对 consul 必然返回
// `coordx: lease driver "consul" not registered`。Consul 的 Session 正是
// 租约原语（可设 TTL、可续期、可销毁），这里补上真实实现。
package consul

import (
	"context"
	"fmt"
	"time"

	"github.com/gospacex/coordx"
	"github.com/gospacex/logx"
	"github.com/gospacex/traceflag"
	"github.com/hashicorp/consul/api"
)

// defaultSessionTTL 未指定 TTL 时使用的会话时长。
const defaultSessionTTL = 30 * time.Second

// minSessionTTL Consul 对 session TTL 有下限约束（过小的 TTL 会被服务端
// 拒绝或导致心跳来不及续期就失效），低于此值一律抬到下限。
const minSessionTTL = 10 * time.Second

// sessionBehavior "delete"：会话失效时 Consul 会自动删除绑定在该 session 上的
// KV，正好对应"租约到期即释放锁"的语义，不会留下僵尸锁。
const sessionBehavior = "delete"

type leaseImpl struct {
	client *api.Client
	cfg    *coordx.Config
	logger logx.Logger
	tracer *traceflag.Tracer
}

// NewLease 创建 Consul 租约（Session）客户端
func NewLease(cfg *coordx.Config) (*leaseImpl, error) {
	addr := "http://localhost:8500"
	if cfg.Addr != "" {
		addr = cfg.Addr
	}
	c, err := api.NewClient(&api.Config{Address: addr})
	if err != nil {
		return nil, fmt.Errorf("coordx/consul: create client: %w", err)
	}
	l := &leaseImpl{
		client: c,
		cfg:    cfg,
		logger: logx.GetGlobal(),
		tracer: traceflag.NewTracer(cfg != nil && cfg.EnableTrace, "github.com/gospacex/coordx/consul", "coordx"),
	}
	l.logger.InfoContext(context.Background(), "coordx/consul lease initialized", logx.String("addr", addr))
	return l, nil
}

// Grant 创建一个带 TTL 的 session，返回其 ID 作为租约 ID。
func (l *leaseImpl) Grant(ctx context.Context, ttl time.Duration) (string, error) {
	ctx, span := startIfParent(ctx, l.tracer, "coord.lease.grant")
	defer span.End()

	if ttl <= 0 {
		ttl = defaultSessionTTL
	}
	if ttl < minSessionTTL {
		ttl = minSessionTTL
	}
	// ⚠️ Consul 的 Create 用 (sessionID, writeMeta, err) 三返回值，且**不会**
	// 因服务端拒绝而返回 err —— 服务端以空 ID 表示拒绝。只判 err 会把
	// "被拒"当成成功（与 nacos RegisterInstance 的坑同源）。
	id, _, err := l.client.Session().Create(&api.SessionEntry{
		Name:     "coordx-lease",
		TTL:      ttl.String(),
		Behavior: sessionBehavior,
	}, nil)
	if err != nil {
		span.RecordError(err)
		l.logger.ErrorContext(ctx, "coordx/consul session create failed", logx.Err(err))
		return "", fmt.Errorf("coordx/consul: session create: %w", err)
	}
	if id == "" {
		err := fmt.Errorf("coordx/consul: session create rejected by server (empty session id)")
		span.RecordError(err)
		l.logger.ErrorContext(ctx, "coordx/consul session create rejected", logx.Err(err))
		return "", err
	}
	l.logger.InfoContext(ctx, "coordx/consul lease granted", logx.String("session_id", id), logx.String("ttl", ttl.String()))
	return id, nil
}

// Renew 续期一次 session。
func (l *leaseImpl) Renew(ctx context.Context, sessionID string) error {
	ctx, span := startIfParent(ctx, l.tracer, "coord.lease.renew")
	defer span.End()

	if sessionID == "" {
		err := fmt.Errorf("coordx/consul: lease renew: empty session id")
		span.RecordError(err)
		return err
	}
	entry, _, err := l.client.Session().Renew(sessionID, nil)
	if err != nil {
		span.RecordError(err)
		l.logger.ErrorContext(ctx, "coordx/consul session renew failed", logx.Err(err), logx.String("session_id", sessionID))
		return fmt.Errorf("coordx/consul: session renew %s: %w", sessionID, err)
	}
	// 同 Grant：服务端用 nil entry 表示"该 session 已不存在"，只判 err 会漏判。
	if entry == nil {
		err := fmt.Errorf("coordx/consul: session renew %s: session not found (expired?)", sessionID)
		span.RecordError(err)
		l.logger.ErrorContext(ctx, "coordx/consul session renew: gone", logx.Err(err))
		return err
	}
	return nil
}

// Revoke 销毁 session（绑定其上的 KV 由 behavior=delete 自动清理）。
func (l *leaseImpl) Revoke(ctx context.Context, sessionID string) error {
	ctx, span := startIfParent(ctx, l.tracer, "coord.lease.revoke")
	defer span.End()

	if sessionID == "" {
		err := fmt.Errorf("coordx/consul: lease revoke: empty session id")
		span.RecordError(err)
		return err
	}
	// ⚠️ consul api 三个 session 方法的返回值形态各不相同，别凭印象套用：
	//   Session().Create  -> (string, *WriteMeta, error)  以空 ID 表示被拒
	//   Session().Renew   -> (*SessionEntry, *WriteMeta, error) 以 nil entry 表示 session 不存在
	//   Session().Destroy -> (*WriteMeta, error)  没有 bool，无法直接分辨"没找到"
	//
	// 因此 Destroy 这里**只判 err**：销毁一个已过期的 session 在语义上等同于
	// 已达成目的（锁已释放），不能因为拿不到 bool 就编一个判断出来。
	if _, err := l.client.Session().Destroy(sessionID, nil); err != nil {
		span.RecordError(err)
		l.logger.ErrorContext(ctx, "coordx/consul session destroy failed", logx.Err(err), logx.String("session_id", sessionID))
		return fmt.Errorf("coordx/consul: session destroy %s: %w", sessionID, err)
	}
	return nil
}

// HealthCheck 与 register/discover 保持一致，返回 nil。
// （同 etcd：三者都没真正探活，属已知技术债，要修就三个一起修。）
func (l *leaseImpl) HealthCheck() error { return nil }
func (l *leaseImpl) Close() error       { return nil }
