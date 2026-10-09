// coordx/etcd/examples: Etcd 服务协调示例
// 功能：Register、Discover、Lease 初始化，HealthCheck
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gospacex/configx"
	"github.com/gospacex/coordx"
	_ "github.com/gospacex/coordx/etcd"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
)

func main() {
	ctx := context.Background()

	if err := logx.ReadConfig(); err != nil {
		log.Printf("logx.ReadConfig fallback: %v", err)
	}
	logger := logx.GetGlobal()
	logger.InfoContext(ctx, "coordx etcd example started", logx.String("driver", "etcd"))

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

	// HealthCheck
	if err := register.HealthCheck(); err != nil {
		logger.Fatal("register health check failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "register health check passed")

	if err := discover.HealthCheck(); err != nil {
		logger.Fatal("discover health check failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "discover health check passed")

	if err := lease.HealthCheck(); err != nil {
		logger.Fatal("lease health check failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "lease health check passed")

	// Register 服务
	instance := &coordx.ServiceInstance{
		ID:        "instance-001",
		ServiceID: "user-service",
		Addr:      "127.0.0.1",
		Port:      8080,
		Metadata:  map[string]string{"version": "1.0.0"},
		Healthy:   true,
	}
	if err := register.Register(ctx, "user-service", instance); err != nil {
		logger.Fatal("register service failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "service registered", logx.String("service", "user-service"))

	// Deregister
	if err := register.Deregister(ctx, "user-service", instance); err != nil {
		logger.ErrorContext(ctx, "deregister failed", logx.Err(err))
	}

	// Discover 服务
	services, err := discover.Discovery(ctx, "user-service")
	if err != nil {
		logger.Fatal("discover failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "discovered services", logx.Int("count", len(services)))

	// Lease Grant
	// ⚠️ 第二个参数是 time.Duration，不是"秒数"。原写法 Grant(ctx, 30) 申请的
	// 是 30 **纳秒**，etcd 会直接拒绝（invalid TTL）。
	sessionID, err := lease.Grant(ctx, 30*time.Second)
	if err != nil {
		logger.Fatal("lease grant failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "lease granted", logx.String("session_id", sessionID))

	// Lease Renew
	if err := lease.Renew(ctx, sessionID); err != nil {
		logger.ErrorContext(ctx, "lease renew failed", logx.Err(err))
	} else {
		logger.InfoContext(ctx, "lease renewed", logx.String("session_id", sessionID))
	}

	// Lease Revoke
	if err := lease.Revoke(ctx, sessionID); err != nil {
		logger.ErrorContext(ctx, "lease revoke failed", logx.Err(err))
	}

	fmt.Println("\n✅ All operations completed successfully.")
}
