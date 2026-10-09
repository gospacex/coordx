// coordx/consul/examples: Consul 服务协调示例
// 功能：注册 → 发现 → 租约（Consul Session）申请/续期/释放 → 注销
//
// 运行：
//
//	cd coordx/consul && ./scripts/start.sh     # 宿主 8500
//	cd examples && go run .
//	cd .. && ./scripts/stop.sh
//
// 若 8500 已被占用：HOST_PORT=<空闲端口> ./scripts/start.sh，并同步改 config.yaml。
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gospacex/configx"
	"github.com/gospacex/coordx"
	_ "github.com/gospacex/coordx/consul"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
)

func main() {
	ctx := context.Background()

	if err := logx.ReadConfig(); err != nil {
		log.Printf("logx.ReadConfig fallback: %v", err)
	}
	logger := logx.GetGlobal()
	logger.InfoContext(ctx, "coordx consul example started", logx.String("driver", "consul"))

	register, discover, lease, err := coordx.ReadAndCreate(configx.ConfigOptions{
		ConfigFile: "config.yaml",
		ConfigType: "yaml",
	})
	if err != nil {
		logger.Fatal("create coordx client failed", logx.Err(err))
	}
	defer func() {
		if cerr := lease.Close(); cerr != nil {
			logger.Error("close lease failed", logx.Err(cerr))
		}
		if cerr := discover.Close(); cerr != nil {
			logger.Error("close discover failed", logx.Err(cerr))
		}
		if cerr := register.Close(); cerr != nil {
			logger.Error("close register failed", logx.Err(cerr))
		}
	}()

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

	const serviceName = "coordx-demo-service"
	inst := &coordx.ServiceInstance{
		ID:        "consul-demo-1",
		ServiceID: serviceName,
		Addr:      "127.0.0.1",
		Port:      8080,
		Metadata:  map[string]string{"version": "1.0.0"},
		Healthy:   true,
	}

	// 注册（驱动内部会起 TTL 心跳维持 passing 状态）
	if err := register.Register(ctx, serviceName, inst); err != nil {
		logger.Fatal("register service failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "service registered", logx.String("service", serviceName))

	// 发现
	found, err := discover.Discovery(ctx, serviceName)
	if err != nil {
		logger.Fatal("discover failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "discovered services", logx.Int("count", len(found)))
	for _, s := range found {
		logger.InfoContext(ctx, "instance", logx.String("id", s.ID), logx.String("addr", s.Addr))
	}

	// 租约：Consul Session 是真实实现（TTL 由驱动抬到最小 10s）
	sessionID, err := lease.Grant(ctx, 30*time.Second)
	if err != nil {
		logger.Fatal("lease grant failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "lease granted", logx.String("session_id", sessionID))

	if err := lease.Renew(ctx, sessionID); err != nil {
		logger.ErrorContext(ctx, "lease renew failed", logx.Err(err))
	} else {
		logger.InfoContext(ctx, "lease renewed")
	}

	if err := lease.Revoke(ctx, sessionID); err != nil {
		logger.ErrorContext(ctx, "lease revoke failed", logx.Err(err))
	} else {
		logger.InfoContext(ctx, "lease revoked")
	}

	// 注销
	if err := register.Deregister(ctx, serviceName, inst); err != nil {
		logger.ErrorContext(ctx, "deregister failed", logx.Err(err))
	} else {
		logger.InfoContext(ctx, "service deregistered")
	}

	fmt.Println("\n✅ All operations completed successfully.")
}
