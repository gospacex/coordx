// coordx/examples/etcd/main.go: etcd 服务协调示例
// 功能：注册、发现、Watch 监听、租约、调用链追踪
package main

import (
	"context"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"log"
	"time"

	"github.com/gospacex/configx"
	"github.com/gospacex/coordx"
	_ "github.com/gospacex/coordx/etcd"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
)

func initTracer() (*sdktrace.TracerProvider, error) {
	exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint("http://localhost:14268/api/traces")))
	if err != nil {
		return nil, fmt.Errorf("jaeger exporter init failed: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.Default()),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	return tp, nil
}

func main() {
	ctx := context.Background()

	// 1. 初始化 logx
	if err := logx.ReadConfig(); err != nil {
		log.Printf("logx.ReadConfig fallback: %v", err)
	}
	logger := logx.GetGlobal()

	p, err := initTracer()
	if err != nil {
		logger.InfoContext(ctx, "tracer init skipped", logx.Err(err))
	} else {
		defer func() { _ = p.Shutdown(ctx) }()
	}
	logger.InfoContext(ctx, "coordx etcd example started", logx.String("driver", "etcd"))

	// 2. 读取配置并创建客户端（注册、发现、租约三个独立实例）
	register, discover, lease, err := coordx.ReadAndCreate(configx.ConfigOptions{
		ConfigFile: "config.yaml",
		ConfigType: "yaml",
	})
	if err != nil {
		logger.Fatal("create coordx client failed", logx.Err(err))
	}
	defer register.Close()
	defer discover.Close()
	defer lease.Close()

	// 3. 健康检查
	for name, hc := range map[string]error{
		"register": register.HealthCheck(),
		"discover": discover.HealthCheck(),
		"lease":    lease.HealthCheck(),
	} {
		if hc != nil {
			logger.Fatal(fmt.Sprintf("%s health check failed", name), logx.Err(hc))
		}
	}
	logger.InfoContext(ctx, "health checks passed")

	serviceName := "test-service"

	// 4. 创建两个服务实例
	instance1 := &coordx.ServiceInstance{
		ID:        "instance-1",
		ServiceID: serviceName,
		Addr:      "127.0.0.1:8081",
		Port:      8081,
		Metadata:  map[string]string{"version": "1.0.0"},
		Healthy:   true,
	}
	instance2 := &coordx.ServiceInstance{
		ID:        "instance-2",
		ServiceID: serviceName,
		Addr:      "127.0.0.1:8082",
		Port:      8082,
		Metadata:  map[string]string{"version": "1.0.1"},
		Healthy:   true,
	}

	// 5. 注册服务
	logger.InfoContext(ctx, "registering services")
	if err := register.Register(ctx, serviceName, instance1); err != nil {
		logger.Fatal("register instance1 failed", logx.Err(err))
	}
	if err := register.Register(ctx, serviceName, instance2); err != nil {
		logger.Fatal("register instance2 failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "services registered", logx.Int("count", 2))

	// 6. 发现服务
	logger.InfoContext(ctx, "discovering service", logx.String("service", serviceName))
	instances, err := discover.Discovery(ctx, serviceName)
	if err != nil {
		logger.Fatal("discover failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "service discovered", logx.Int("count", len(instances)))
	for _, inst := range instances {
		logger.InfoContext(ctx, "instance", logx.String("id", inst.ID), logx.String("addr", inst.Addr))
	}

	// 7. Watch 服务变更监听
	stopCh := make(chan struct{})
	go func() {
		if err := discover.Watch(ctx, serviceName, func(insts []*coordx.ServiceInstance) {
			logger.InfoContext(ctx, "service watch triggered", logx.Int("count", len(insts)))
			for _, inst := range insts {
				logger.InfoContext(ctx, "watch instance", logx.String("id", inst.ID), logx.String("addr", inst.Addr))
			}
		}); err != nil {
			logger.ErrorContext(ctx, "watch error", logx.Err(err))
		}
	}()

	// 等待 watch 回调
	time.Sleep(500 * time.Millisecond)
	close(stopCh)
	logger.InfoContext(ctx, "watch test completed")

	// 8. 租约管理
	logger.InfoContext(ctx, "creating lease", logx.Int("ttl_sec", 30))
	sessionID, err := lease.Grant(ctx, 30*time.Second)
	if err != nil {
		logger.Fatal("create lease failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "lease created", logx.String("session_id", sessionID))

	// 9. 续期租约
	logger.InfoContext(ctx, "renewing lease")
	if err := lease.Renew(ctx, sessionID); err != nil {
		logger.Fatal("renew lease failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "lease renewed")

	// 10. 撤销租约
	logger.InfoContext(ctx, "revoking lease")
	if err := lease.Revoke(ctx, sessionID); err != nil {
		logger.Fatal("revoke lease failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "lease revoked")

	// 11. 注销服务
	logger.InfoContext(ctx, "deregistering services")
	if err := register.Deregister(ctx, serviceName, instance1); err != nil {
		logger.Fatal("deregister instance1 failed", logx.Err(err))
	}
	if err := register.Deregister(ctx, serviceName, instance2); err != nil {
		logger.Fatal("deregister instance2 failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "services deregistered")

	fmt.Println("\n✅ All operations completed successfully.")
}
