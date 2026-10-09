package nacos

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gospacex/coordx"
	"github.com/gospacex/logx"
	"github.com/gospacex/traceflag"
	"go.opentelemetry.io/otel/trace"
)

// parentCtx 构造一个带**有效**父 span 的 ctx。
// startIfParent 只在有父 span 时才真正调用 tracer.Start，所以想覆盖到 tracer
// 那条路径（而不是被 noop 短路），必须喂这种 ctx。
func parentCtx() context.Context {
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x01},
		SpanID:     trace.SpanID{0x01},
		TraceFlags: trace.FlagsSampled,
	})
	return trace.ContextWithSpanContext(context.Background(), sc)
}

// ===== Lease：Nacos 无租约原语，三个方法必须显式报错而非静默成功 =====

func TestLeaseUnsupported(t *testing.T) {
	cfg := &coordx.Config{Driver: "nacos", Addr: "127.0.0.1:8848"}
	l, err := NewLease(cfg)
	if err != nil {
		t.Fatalf("NewLease: %v", err)
	}
	ctx := parentCtx()

	if id, err := l.Grant(ctx, 30*time.Second); err == nil {
		t.Fatal("Grant 应返回 error，实际 nil（Nacos 不支持 lease）")
	} else if !errors.Is(err, ErrLeaseUnsupported) {
		t.Errorf("Grant 错误应可被 errors.Is(ErrLeaseUnsupported) 判定，实际: %v", err)
	} else if id != "" {
		t.Errorf("Grant 失败时 sessionID 应为空串，实际 %q", id)
	}

	// ⚠ 这两个是最危险的：修复前它们返回 nil，调用方会误以为续约/释放成功。
	if err := l.Renew(ctx, "sess-1"); err == nil {
		t.Fatal("Renew 应返回 error，实际 nil —— 返回 nil 会让调用方误以为续约成功")
	} else if !errors.Is(err, ErrLeaseUnsupported) {
		t.Errorf("Renew 错误应可被 errors.Is 判定，实际: %v", err)
	}
	if err := l.Revoke(ctx, "sess-1"); err == nil {
		t.Fatal("Revoke 应返回 error，实际 nil —— 返回 nil 会让调用方误以为释放成功")
	} else if !errors.Is(err, ErrLeaseUnsupported) {
		t.Errorf("Revoke 错误应可被 errors.Is 判定，实际: %v", err)
	}
}

// ===== Register / Deregister：nil instance 必须在触碰 client 前就被拦下 =====

func TestRegisterNilInstance(t *testing.T) {
	// client 故意留 nil：若 nil 判断被挪到 client 调用之后，这里会 panic。
	r := &registerImpl{
		cfg:    &coordx.Config{Driver: "nacos"},
		logger: logx.GetGlobal(),
		tracer: traceflag.NewTracer(false, "test", "coordx"),
	}
	if err := r.Register(parentCtx(), "svc-a", nil); err == nil {
		t.Fatal("Register(nil) 应返回 error，实际 nil")
	}
	if err := r.Deregister(parentCtx(), "svc-a", nil); err == nil {
		t.Fatal("Deregister(nil) 应返回 error，实际 nil")
	}
}

// TestRegisterTracerInitialized 钉住 NewRegister 必须初始化 tracer。
// 修复前 NewRegister 漏了 tracer 字段，而 Register 会调 r.tracer.Start ——
// nil receiver 在 traceflag.Tracer.Start → Enabled 里解引用 t.local 直接 panic。
// 由于 startIfParent 无父时短路，启动期恰好绕开了 panic，但 Register 永远
// 产不出 span；一旦调用方在请求上下文里注册就会崩。
func TestRegisterTracerInitialized(t *testing.T) {
	r, err := NewRegister(&coordx.Config{Driver: "nacos", Addr: "127.0.0.1:8848", Cluster: "test-ns"})
	if err != nil {
		t.Fatalf("NewRegister: %v", err)
	}
	if r.tracer == nil {
		t.Fatal("NewRegister 未初始化 tracer：Register/Deregister 会在 nil receiver 上 panic")
	}
	// 带父 span 的 ctx 会走 tracer.Start 真实路径，能同时钉住"不 panic"。
	// 这里 client 已建立但服务端不可达，nil instance 会在网络调用前返回。
	if err := r.Register(parentCtx(), "svc-a", nil); err == nil {
		t.Fatal("Register(nil) 应返回 error")
	}
}

// ===== parseServerConfig：纯函数，钉住地址解析契约 =====

func TestParseServerConfig(t *testing.T) {
	cases := []struct {
		name string
		addr string
		ip   string
		port uint64
	}{
		{"空地址回落默认", "", "", 8848},
		{"host:port", "nacos:8848", "nacos", 8848},
		{"带 http scheme", "http://nacos:9848", "nacos", 9848},
		{"带路径", "http://nacos:8848/nacos", "nacos", 8848},
		{"仅 host 无端口", "nacos", "nacos", 8848},
		{"端口非数字", "nacos:abc", "nacos:abc", 8848},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseServerConfig(c.addr)
			if got.IpAddr != c.ip || got.Port != c.port {
				t.Errorf("parseServerConfig(%q) = (%q, %d), want (%q, %d)",
					c.addr, got.IpAddr, got.Port, c.ip, c.port)
			}
			if got.ContextPath != "/nacos" || got.Scheme != "http" {
				t.Errorf("parseServerConfig(%q) 应保留 ContextPath=/nacos Scheme=http，实际 %q/%q",
					c.addr, got.ContextPath, got.Scheme)
			}
		})
	}
}

// ===== 驱动注册契约 =====

func TestDriverRegistered(t *testing.T) {
	for _, kind := range []struct {
		name string
		drv  string
		ok   bool
	}{
		{"register", "nacos", true},
		{"discover", "nacos", true},
		{"lease", "nacos", true},
		{"unknown", "no-such-driver", false},
	} {
		cfg := &coordx.Config{Driver: kind.drv, Addr: "127.0.0.1:8848"}
		var err error
		switch kind.name {
		case "register":
			_, err = coordx.NewRegister(cfg)
		case "discover":
			_, err = coordx.NewDiscover(cfg)
		case "lease":
			_, err = coordx.NewLease(cfg)
		default:
			_, err = coordx.NewRegister(cfg)
		}
		if kind.ok && err != nil {
			t.Errorf("%s driver %q 已注册，New* 不该报未注册错误，实际: %v", kind.name, kind.drv, err)
		}
		if !kind.ok && err == nil {
			t.Errorf("%s driver %q 未注册，应返回 error，实际 nil", kind.name, kind.drv)
		}
	}
}
