package apollo

import (
	"context"
	"errors"
	"testing"

	"github.com/gospacex/coordx"
)

// ===== Register / Deregister：Apollo 无注册能力，必须显式报错 =====

func TestRegisterUnsupported(t *testing.T) {
	r, err := NewRegister(&coordx.Config{Driver: "apollo", Addr: "http://127.0.0.1:8070"})
	if err != nil {
		t.Fatalf("NewRegister: %v", err)
	}
	inst := &coordx.ServiceInstance{ID: "inst-1", Addr: "10.0.0.1", Port: 9000}

	if err := r.Register(context.Background(), "svc-a", inst); err == nil {
		t.Fatal("Register 应返回 error（Apollo 无服务注册能力），实际 nil")
	} else if !errors.Is(err, ErrRegisterUnsupported) {
		t.Errorf("Register 错误应可被 errors.Is(ErrRegisterUnsupported) 判定，实际: %v", err)
	} else if !errors.Is(err, coordx.ErrUnsupported) {
		t.Errorf("Register 错误也应可被 errors.Is(coordx.ErrUnsupported) 统一判定，实际: %v", err)
	}

	if err := r.Deregister(context.Background(), "svc-a", inst); err == nil {
		t.Fatal("Deregister 应返回 error，实际 nil")
	} else if !errors.Is(err, ErrRegisterUnsupported) {
		t.Errorf("Deregister 错误应可被 errors.Is 判定，实际: %v", err)
	}
}

// TestRegisterNilInstance 钉住 nil instance 不会 panic。
// 修复前 SetAttributes 里的 inst.ID 排在 nil 判断之前，直接解引用 panic。
func TestRegisterNilInstance(t *testing.T) {
	r, err := NewRegister(&coordx.Config{Driver: "apollo"})
	if err != nil {
		t.Fatalf("NewRegister: %v", err)
	}
	if err := r.Register(context.Background(), "svc-a", nil); err == nil {
		t.Fatal("Register(nil) 应返回 error 而非 panic")
	}
	if err := r.Deregister(context.Background(), "svc-a", nil); err == nil {
		t.Fatal("Deregister(nil) 应返回 error 而非 panic")
	}
}

// TestNewRegisterNilConfig 钉住 cfg 为 nil 时不panic（此前 cfg.Addr 直接解引用）。
func TestNewRegisterNilConfig(t *testing.T) {
	r, err := NewRegister(nil) //nolint:staticcheck // 故意传 nil，验证健壮性
	if err != nil {
		t.Fatalf("NewRegister(nil) 不应报错，实际: %v", err)
	}
	if r == nil {
		t.Fatal("NewRegister(nil) 应返回非 nil 实例")
	}
}

// ===== 驱动注册契约 =====

func TestDriverRegistered(t *testing.T) {
	cfg := &coordx.Config{Driver: "apollo", Addr: "http://127.0.0.1:8070"}
	if _, err := coordx.NewRegister(cfg); err != nil {
		t.Errorf("apollo 驱动已注册，NewRegister 不该报未注册错误，实际: %v", err)
	}
	unknown := &coordx.Config{Driver: "no-such-driver"}
	if _, err := coordx.NewRegister(unknown); err == nil {
		t.Error("未注册驱动应返回 error，实际 nil")
	}
}
