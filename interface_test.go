package coordx

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// ===== 纯逻辑：Config.FillDefaults =====

func TestConfigFillDefaults(t *testing.T) {
	c := &Config{Driver: "consul"}
	c.FillDefaults()
	if c.Name != "default" {
		t.Errorf("Name = %q, want %q (空 Name 应填默认值)", c.Name, "default")
	}

	c2 := &Config{Driver: "consul", Name: "svc-a"}
	c2.FillDefaults()
	if c2.Name != "svc-a" {
		t.Errorf("Name = %q, want %q (已有 Name 不应被覆盖)", c2.Name, "svc-a")
	}
}

// ===== 工厂：未注册 driver 必须返回明确错误，而不是 nil/panic =====

func TestNewRegisterUnregisteredDriver(t *testing.T) {
	_, err := NewRegister(&Config{Driver: "definitely-not-registered-register"})
	if err == nil {
		t.Fatal("未注册 driver 应返回 error，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("错误信息应含 %q 便于定位，实际: %v", "not registered", err)
	}
}

func TestNewDiscoverUnregisteredDriver(t *testing.T) {
	_, err := NewDiscover(&Config{Driver: "definitely-not-registered-discover"})
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("未注册 driver 应返回含 not registered 的 error，实际: %v", err)
	}
}

func TestNewLeaseUnregisteredDriver(t *testing.T) {
	_, err := NewLease(&Config{Driver: "definitely-not-registered-lease"})
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("未注册 driver 应返回含 not registered 的 error，实际: %v", err)
	}
}

// ===== 工厂：注册后能创建（钉住 creator 契约不被后续重构破坏）=====

// fakeCoord 同时实现 Register/Discover/Lease，仅用于验证工厂接线是否正确。
type fakeCoord struct{ cfg *Config }

func (f *fakeCoord) Register(ctx context.Context, service string, i *ServiceInstance) error {
	return nil
}
func (f *fakeCoord) Deregister(ctx context.Context, service string, i *ServiceInstance) error {
	return nil
}
func (f *fakeCoord) Discovery(ctx context.Context, service string) ([]*ServiceInstance, error) {
	return nil, nil
}
func (f *fakeCoord) Watch(ctx context.Context, service string, cb func([]*ServiceInstance)) error {
	return nil
}
func (f *fakeCoord) Grant(ctx context.Context, ttl time.Duration) (string, error) { return "", nil }
func (f *fakeCoord) Renew(ctx context.Context, sessionID string) error            { return nil }
func (f *fakeCoord) Revoke(ctx context.Context, sessionID string) error           { return nil }
func (f *fakeCoord) HealthCheck() error                                           { return nil }
func (f *fakeCoord) Close() error                                                 { return nil }

func TestFactoryCreateAfterRegister(t *testing.T) {
	const drv = "test-factory-coordx"
	RegisterCreator(drv, func(c *Config) (Register, error) { return &fakeCoord{cfg: c}, nil })
	RegisterDiscoverCreator(drv, func(c *Config) (Discover, error) { return &fakeCoord{cfg: c}, nil })
	RegisterLeaseCreator(drv, func(c *Config) (Lease, error) { return &fakeCoord{cfg: c}, nil })

	cfg := &Config{Driver: drv}
	if _, err := NewRegister(cfg); err != nil {
		t.Errorf("NewRegister: %v", err)
	}
	if _, err := NewDiscover(cfg); err != nil {
		t.Errorf("NewDiscover: %v", err)
	}
	if _, err := NewLease(cfg); err != nil {
		t.Errorf("NewLease: %v", err)
	}
}

// TestFactoryConcurrentAccess 钉住 factory map 的并发安全性。
//
// 背景：REVIEW-REPORT.md 第二优先级第 6 条要求「所有 factory map 添加
// sync.RWMutex」，列举了 coordx；但第八节已修复清单只记了 mqx/sqlx/kvx/
// searchx，coordx 被漏掉。本用例在 -race 下并发注册 + 并发创建，若 map 无
// 保护会直接报 DATA RACE。
//
// 说明：驱动注册通常在 init() 单线程完成，运行时只读，因此这条竞态在生产
// 上不常见；但热加载/插件化注册场景会命中，且修复成本极低。
func TestFactoryConcurrentAccess(t *testing.T) {
	const n = 32
	var wg sync.WaitGroup
	wg.Add(n * 2)

	for i := 0; i < n; i++ {
		drv := "concurrent-coordx-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		go func(d string) {
			defer wg.Done()
			RegisterCreator(d, func(c *Config) (Register, error) { return &fakeCoord{cfg: c}, nil })
			RegisterDiscoverCreator(d, func(c *Config) (Discover, error) { return &fakeCoord{cfg: c}, nil })
			RegisterLeaseCreator(d, func(c *Config) (Lease, error) { return &fakeCoord{cfg: c}, nil })
		}(drv)

		// 并发读：命中与否都允许，只要不触发 race / panic
		go func(d string) {
			defer wg.Done()
			_, _ = NewRegister(&Config{Driver: d})
			_, _ = NewDiscover(&Config{Driver: d})
			_, _ = NewLease(&Config{Driver: d})
		}(drv)
	}
	wg.Wait()
}
