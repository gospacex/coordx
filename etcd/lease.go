// coordx/etcd: 分布式租约实现（etcd Lease）
//
// 背景：coordx.ReadAndCreate 按"三件套"契约依次创建 Register / Discover /
// Lease，而本驱动此前只注册了前两个，导致统一入口对 etcd 必然返回
// `coordx: lease driver "etcd" not registered` —— etcd 明明有原生 Lease
// 能力却用不了（consul 同理）。这里补上真实实现，而不是占位。
package etcd

import (
	"context"
	"crypto/tls"
	"fmt"
	"strconv"
	"time"

	"github.com/gospacex/coordx"
	"github.com/gospacex/logx"
	"github.com/gospacex/traceflag"
	"go.etcd.io/etcd/client/v3"
)

// defaultLeaseTTL Grant 未给 TTL（或给了非正值）时使用的租约时长。
const defaultLeaseTTL = 30 * time.Second

type leaseImpl struct {
	client *clientv3.Client
	cfg    *coordx.Config
	logger logx.Logger
	tracer *traceflag.Tracer
}

// NewLease 创建 etcd 租约客户端
func NewLease(cfg *coordx.Config) (*leaseImpl, error) {
	ctx := context.Background()
	endpoints := []string{"http://localhost:2379"}
	if cfg.Addr != "" {
		endpoints = []string{cfg.Addr}
	}
	etcdCfg := clientv3.Config{Endpoints: endpoints, DialTimeout: 5 * time.Second}
	if cfg.TLSEnabled {
		etcdCfg.TLS = &tls.Config{InsecureSkipVerify: true}
	}
	var c *clientv3.Client
	if err := withRetry(func() error {
		var err2 error
		c, err2 = clientv3.New(etcdCfg)
		return err2
	}, 3); err != nil {
		return nil, fmt.Errorf("coordx/etcd: connect: %w", err)
	}
	l := &leaseImpl{
		client: c,
		cfg:    cfg,
		logger: logx.GetGlobal(),
		tracer: traceflag.NewTracer(cfg != nil && cfg.EnableTrace, "github.com/gospacex/coordx/etcd", "coordx"),
	}
	l.logger.InfoContext(ctx, "coordx/etcd lease initialized", logx.String("addr", cfg.Addr))
	return l, nil
}

// Grant 申请一个 TTL 租约，返回字符串形式的 etcd LeaseID。
//
// sessionID 之所以用字符串：coordx.Lease 接口返回的 ID 要能透传给
// Renew/Revoke，也可能被业务侧落到配置里，数字 ID 直接暴露会耦合 etcd 类型。
// 这里用十进制字符串，Renew/Revoke 再解析回去。
func (l *leaseImpl) Grant(ctx context.Context, ttl time.Duration) (string, error) {
	ctx, span := startIfParent(ctx, l.tracer, "coord.lease.grant")
	defer span.End()

	if ttl <= 0 {
		ttl = defaultLeaseTTL
	}
	// etcd 的 Grant 以秒为单位，不足 1s 会被服务端拒绝（invalid TTL），
	// 这里向上取整到 1s，避免"传了 500ms 换来一个报错"。
	secs := int64(ttl.Seconds())
	if secs < 1 {
		secs = 1
	}
	resp, err := l.client.Grant(ctx, secs)
	if err != nil {
		span.RecordError(err)
		l.logger.ErrorContext(ctx, "coordx/etcd lease grant failed", logx.Err(err))
		return "", fmt.Errorf("coordx/etcd: lease grant: %w", err)
	}
	id := strconv.FormatInt(int64(resp.ID), 10)
	l.logger.InfoContext(ctx, "coordx/etcd lease granted", logx.String("lease_id", id), logx.Int("ttl_sec", int(secs)))
	return id, nil
}

// Renew 续约一次（KeepAliveOnce：立即续一次并刷新 TTL）。
func (l *leaseImpl) Renew(ctx context.Context, sessionID string) error {
	ctx, span := startIfParent(ctx, l.tracer, "coord.lease.renew")
	defer span.End()

	id, err := parseLeaseID(sessionID)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if _, err := l.client.KeepAliveOnce(ctx, clientv3.LeaseID(id)); err != nil {
		span.RecordError(err)
		l.logger.ErrorContext(ctx, "coordx/etcd lease renew failed", logx.Err(err), logx.String("lease_id", sessionID))
		return fmt.Errorf("coordx/etcd: lease renew %s: %w", sessionID, err)
	}
	return nil
}

// Revoke 立即释放租约（绑定在该租约上的 key 会被 etcd 同步删除）。
func (l *leaseImpl) Revoke(ctx context.Context, sessionID string) error {
	ctx, span := startIfParent(ctx, l.tracer, "coord.lease.revoke")
	defer span.End()

	id, err := parseLeaseID(sessionID)
	if err != nil {
		span.RecordError(err)
		return err
	}
	if _, err := l.client.Revoke(ctx, clientv3.LeaseID(id)); err != nil {
		span.RecordError(err)
		l.logger.ErrorContext(ctx, "coordx/etcd lease revoke failed", logx.Err(err), logx.String("lease_id", sessionID))
		return fmt.Errorf("coordx/etcd: lease revoke %s: %w", sessionID, err)
	}
	return nil
}

// parseLeaseID 解析 Grant 返回的字符串 ID；非法输入返回明确错误，
// 不要让 strconv 的零值静默变成 LeaseID(0)（那会误操作到"0 号租约"）。
func parseLeaseID(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("coordx/etcd: empty lease id")
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("coordx/etcd: invalid lease id %q: %w", s, err)
	}
	if id <= 0 {
		return 0, fmt.Errorf("coordx/etcd: invalid lease id %q (must be positive)", s)
	}
	return id, nil
}

// HealthCheck 与 register/discover 保持一致，返回 nil。
//
// ⚠️ 已知技术债：三者都没有真正探活（构造时 clientv3.New 已建连，但
// HealthCheck 不检查连通性）。这里刻意不擅自改成真实探活——那会让三件套
// 的 HealthCheck 行为在同一驱动内出现差异。若要修，应三个一起修。
func (l *leaseImpl) HealthCheck() error { return nil }
func (l *leaseImpl) Close() error       { return l.client.Close() }
