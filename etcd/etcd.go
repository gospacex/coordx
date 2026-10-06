// coordx/etcd: etcd 服务注册与发现驱动
package etcd

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/gospacex/coordx"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
	"github.com/gospacex/traceflag"
	"go.etcd.io/etcd/client/v3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type registerImpl struct {
	client *clientv3.Client
	cfg    *coordx.Config
	logger logx.Logger
	tracer *traceflag.Tracer
}

type discoverImpl struct {
	client *clientv3.Client
	cfg    *coordx.Config
	logger logx.Logger
	tracer *traceflag.Tracer
}

// NewRegister 创建 etcd 注册中心
func NewRegister(cfg *coordx.Config) (*registerImpl, error) {
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
	tracer := traceflag.NewTracer(cfg != nil && cfg.EnableTrace, "github.com/gospacex/coordx/etcd", "coordx")
	r := &registerImpl{client: c, cfg: cfg, logger: logx.GetGlobal(), tracer: tracer}
	r.logger.InfoContext(ctx, "coordx/etcd register initialized", logx.String("addr", cfg.Addr))
	_, span := tracer.Start(ctx, "coordx.etcd.register.init")
	defer span.End()
	span.SetAttributes(attribute.String("coordx.driver", "etcd"))
	return r, nil
}

func (r *registerImpl) Register(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	ctx, span := r.tracer.Start(ctx, "coord.register")
	defer span.End()
	span.SetAttributes(
		attribute.String("coord.driver", "etcd"),
		attribute.String("coord.service", service),
		attribute.String("coord.instance_id", inst.ID),
	)
	key := fmt.Sprintf("/services/%s/%s", service, inst.ID)
	span.SetAttributes(attribute.String("coord.key", key))
	leaseResp, err := r.client.Grant(ctx, 30)
	if err != nil {
		span.RecordError(err)
		return err
	}
	_, err = r.client.Put(ctx, key, inst.Addr, clientv3.WithLease(leaseResp.ID))
	if err != nil {
		span.RecordError(err)
		r.logger.ErrorContext(ctx, "coordx/etcd register failed", logx.Err(err), logx.String("service", service), logx.String("inst_id", inst.ID))
		return err
	}
	_, err = r.client.KeepAlive(ctx, leaseResp.ID)
	if err != nil {
		span.RecordError(err)
		r.logger.ErrorContext(ctx, "coordx/etcd keepalive failed", logx.Err(err), logx.String("service", service))
		return err
	}
	r.logger.InfoContext(ctx, "coordx/etcd register done", logx.String("service", service), logx.String("inst_id", inst.ID))
	return nil
}
func (r *registerImpl) Deregister(ctx context.Context, service string, inst *coordx.ServiceInstance) error {
	ctx, span := r.tracer.Start(ctx, "coord.deregister")
	defer span.End()
	span.SetAttributes(
		attribute.String("coord.driver", "etcd"),
		attribute.String("coord.service", service),
		attribute.String("coord.instance_id", inst.ID),
	)
	key := fmt.Sprintf("/services/%s/%s", service, inst.ID)
	span.SetAttributes(attribute.String("coord.key", key))
	_, err := r.client.Delete(ctx, key)
	if err != nil {
		span.RecordError(err)
		r.logger.ErrorContext(ctx, "coordx/etcd deregister failed", logx.Err(err), logx.String("service", service))
		return err
	}
	r.logger.InfoContext(ctx, "coordx/etcd deregister done", logx.String("service", service), logx.String("inst_id", inst.ID))
	return nil
}
func (r *registerImpl) HealthCheck() error { return nil }
func (r *registerImpl) Close() error       { return r.client.Close() }

// NewDiscover 创建 etcd 发现器
func NewDiscover(cfg *coordx.Config) (*discoverImpl, error) {
	ctx := context.Background()
	endpoints := []string{"http://localhost:2379"}
	if cfg.Addr != "" {
		endpoints = []string{cfg.Addr}
	}
	etcdCfg := clientv3.Config{Endpoints: endpoints, DialTimeout: 5 * time.Second}
	if cfg.TLSEnabled {
		etcdCfg.TLS = &tls.Config{InsecureSkipVerify: true}
	}
	c, err := clientv3.New(etcdCfg)
	if err != nil {
		return nil, fmt.Errorf("coordx/etcd: connect: %w", err)
	}
	tracer := traceflag.NewTracer(cfg != nil && cfg.EnableTrace, "github.com/gospacex/coordx/etcd", "coordx")
	d := &discoverImpl{client: c, cfg: cfg, logger: logx.GetGlobal(), tracer: tracer}
	d.logger.InfoContext(ctx, "coordx/etcd discover initialized", logx.String("addr", cfg.Addr))
	_, span := tracer.Start(ctx, "coordx.etcd.discover.init")
	defer span.End()
	span.SetAttributes(attribute.String("coordx.driver", "etcd"))
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
		attribute.String("coord.driver", "etcd"),
		attribute.String("coord.service", service),
	)
	resp, err := d.client.Get(ctx, fmt.Sprintf("/services/%s/", service), clientv3.WithPrefix())
	if err != nil {
		span.RecordError(err)
		return nil, err
	}
	var result []*coordx.ServiceInstance
	for range resp.Kvs {
		// TODO: decode ServiceInstance JSON from kv.Value
		result = append(result, &coordx.ServiceInstance{})
	}
	span.SetAttributes(attribute.Int("coord.instance_count", len(result)))
	d.logger.InfoContext(ctx, "coordx/etcd discovery done", logx.String("service", service), logx.Int("instances", len(result)))
	return result, nil
}
func (d *discoverImpl) Watch(ctx context.Context, service string, callback func([]*coordx.ServiceInstance)) error {
	ctx, span := d.tracer.Start(ctx, "coord.watch")
	defer span.End()
	span.SetAttributes(
		attribute.String("coord.driver", "etcd"),
		attribute.String("coord.service", service),
	)
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	d.logger.InfoContext(ctx, "coordx/etcd watch started", logx.String("service", service))
	ch := d.client.Watch(watchCtx, fmt.Sprintf("/services/%s/", service), clientv3.WithPrefix())
	go func() {
		for range ch {
			insts, _ := d.Discovery(ctx, service)
			if callback != nil {
				callback(insts)
			}
		}
	}()
	return nil
}
func (d *discoverImpl) HealthCheck() error { return nil }
func (d *discoverImpl) Close() error       { return d.client.Close() }

func init() {
	coordx.RegisterCreator("etcd", func(cfg *coordx.Config) (coordx.Register, error) {
		return NewRegister(cfg)
	})
	coordx.RegisterDiscoverCreator("etcd", func(cfg *coordx.Config) (coordx.Discover, error) {
		return NewDiscover(cfg)
	})
}

func withRetry(fn func() error, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			time.Sleep(time.Duration(100<<uint(i-1)) * time.Millisecond)
		}
		if lastErr = fn(); lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("failed to connect after %d retries: %w", maxRetries, lastErr)
}
