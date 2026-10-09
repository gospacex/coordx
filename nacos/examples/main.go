// coordx/nacos/examples: Nacos 服务注册与发现示例
// 功能：注册（ephemeral 临时实例）→ 发现 → 注销，并演示 Lease 的能力降级
//
// 运行：
//
//	cd coordx/nacos && ./scripts/start.sh      # 宿主 8848（Nacos 启动较慢，脚本会等待就绪）
//	cd examples && go run .
//	cd .. && ./scripts/stop.sh
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/gospacex/configx"
	"github.com/gospacex/coordx"
	_ "github.com/gospacex/coordx/nacos"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
)

func main() {
	ctx := context.Background()

	if err := logx.ReadConfig(); err != nil {
		log.Printf("logx.ReadConfig fallback: %v", err)
	}
	logger := logx.GetGlobal()
	logger.InfoContext(ctx, "coordx nacos example started", logx.String("driver", "nacos"))

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
	} {
		if hc != nil {
			logger.Fatal(fmt.Sprintf("%s health check failed", name), logx.Err(hc))
		}
	}
	logger.InfoContext(ctx, "register/discover health checks passed")

	const serviceName = "coordx-demo-service"
	inst := &coordx.ServiceInstance{
		ID:        "nacos-demo-1",
		ServiceID: serviceName,
		Addr:      "127.0.0.1",
		Port:      8080,
		Metadata:  map[string]string{"version": "1.0.0"},
		Healthy:   true,
	}

	// 注册：Nacos 用 ephemeral 临时实例，进程退出后由服务端心跳超时自动摘除。
	// 驱动内部检查了 RegisterInstance 的 (bool, error) —— Nacos 会以
	// (false, nil) 表示服务端拒绝，只判 err 会把它当成功吞掉。
	if err := register.Register(ctx, serviceName, inst); err != nil {
		logger.Fatal("register service failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "service registered", logx.String("service", serviceName))

	found, err := discover.Discovery(ctx, serviceName)
	if err != nil {
		logger.Fatal("discover failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "discovered services", logx.Int("count", len(found)))
	for _, s := range found {
		logger.InfoContext(ctx, "instance", logx.String("id", s.ID), logx.String("addr", s.Addr))
	}

	// Lease：Nacos 是心跳模型，没有租约原语，驱动显式返回 ErrUnsupported。
	// 这里演示正确的降级姿势——用 errors.Is 判定后跳过，而不是当成致命错误。
	if _, err := lease.Grant(ctx, 30*time.Second); err != nil {
		if errors.Is(err, coordx.ErrUnsupported) {
			logger.InfoContext(ctx, "lease 不被 nacos 支持，按设计降级跳过", logx.Err(err))
		} else {
			logger.Fatal("lease grant failed with unexpected error", logx.Err(err))
		}
	}

	// 注销
	if err := register.Deregister(ctx, serviceName, inst); err != nil {
		logger.ErrorContext(ctx, "deregister failed", logx.Err(err))
	} else {
		logger.InfoContext(ctx, "service deregistered")
	}

	fmt.Println("\n✅ All operations completed successfully.")
}
