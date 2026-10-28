package consul

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gospacex/coordx"
	"github.com/hashicorp/consul/api"
)

// TestIsNotFound 表驱动覆盖 404 判定的全部输入形态。
// 值类型匹配（StatusError 非指针）+ wrap 传递是重点：consul api 各版本
// 可能给错误套上下文，只认裸值会在包装后漏判。
func TestIsNotFound(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"bare 404", api.StatusError{Code: http.StatusNotFound, Body: "Service not found"}, true},
		{"wrapped 404", fmt.Errorf("coordx/consul: deregister: %w", api.StatusError{Code: http.StatusNotFound}), true},
		{"double wrapped 404", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", api.StatusError{Code: 404})), true},
		{"500 is not notfound", api.StatusError{Code: http.StatusInternalServerError, Body: "boom"}, false},
		{"plain error", errors.New("dial tcp: connection refused"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isNotFound(c.err); got != c.want {
				t.Fatalf("isNotFound(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

// newFakeConsul 起一个假 consul agent：按 status 返回 deregister 的响应码。
func newFakeConsul(t *testing.T, status int, hits *int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		w.WriteHeader(status)
	}))
}

// TestDeregisterAlreadyGoneReturnsNil 回归 P1：双写注册（launchx stepRegister
// + kitex WithRegistry，同一 instance ID）下第二条注销路径必然吃 404。
// 契约是「确保实例不在注册中心」——已不在即满足，必须返回 nil，
// 否则 launchx Close 报错、进程优雅停机退 1。
func TestDeregisterAlreadyGoneReturnsNil(t *testing.T) {
	var hits int32
	srv := newFakeConsul(t, http.StatusNotFound, &hits)
	defer srv.Close()

	r, err := NewRegister(&coordx.Config{Driver: "consul", Addr: srv.URL})
	if err != nil {
		t.Fatalf("NewRegister: %v", err)
	}
	// 预置一条心跳，验证 404 容错路径同样清掉 beats（泄漏会让
	// registerImpl.Close 之后仍有 goroutine 引用 map）。
	r.beats["srv-x-127.0.0.1-9021"] = make(chan struct{})

	err = r.Deregister(context.Background(), "srv-x", &coordx.ServiceInstance{
		ID: "srv-x-127.0.0.1-9021", ServiceID: "srv-x-127.0.0.1-9021",
	})
	if err != nil {
		t.Fatalf("Deregister on 404 = %v, want nil", err)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Fatalf("fake consul hits = %d, want 1", n)
	}
	r.mu.Lock()
	left := len(r.beats)
	r.mu.Unlock()
	if left != 0 {
		t.Fatalf("beats left = %d, want 0 (heartbeat leaked)", left)
	}
}

// TestDeregisterServerErrorStillFails 反向对照：非 404（500）必须照旧报错，
// 容错不能把真故障一起吞了。
func TestDeregisterServerErrorStillFails(t *testing.T) {
	srv := newFakeConsul(t, http.StatusInternalServerError, nil)
	defer srv.Close()

	r, err := NewRegister(&coordx.Config{Driver: "consul", Addr: srv.URL})
	if err != nil {
		t.Fatalf("NewRegister: %v", err)
	}
	err = r.Deregister(context.Background(), "srv-x", &coordx.ServiceInstance{ID: "srv-x-1"})
	if err == nil {
		t.Fatal("Deregister on 500 = nil, want error")
	}
}
