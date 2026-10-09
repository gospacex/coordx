// coordx/apollo/examples: Apollo 驱动的能力边界示例
//
// ⚠️ 这个示例的教学意义大于功能演示：Apollo 是**配置中心**，其开放 API 只有
// 配置发布/拉取，没有服务注册、发现、租约这组原语。因此本驱动的三个能力
// 全部返回 wrap 了 coordx.ErrUnsupported 的具名错误。
//
// 示例演示的是**正确的降级姿势**：用 errors.Is(err, coordx.ErrUnsupported)
// 判定后跳过，而不是：
//   - 把 err 当致命错误中断（会误伤"本来就该降级"的场景）；
//   - 忽略 err（此前 Lease.Renew 返回 nil 假装成功，比报错危险得多）。
//
// 运行（不需要真的连 Apollo，驱动不发任何请求）：
//
//	cd coordx/apollo/examples && go run .
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/gospacex/configx"
	"github.com/gospacex/coordx"
	_ "github.com/gospacex/coordx/apollo"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
)

func main() {
	ctx := context.Background()

	if err := logx.ReadConfig(); err != nil {
		log.Printf("logx.ReadConfig fallback: %v", err)
	}
	logger := logx.GetGlobal()
	logger.InfoContext(ctx, "coordx apollo example started", logx.String("driver", "apollo"))

	// 统一入口现在能走通了：driver: apollo 的三项能力都已注册（此前缺
	// Discover/Lease，ReadAndCreate 会直接报 "lease driver not registered"）。
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
	logger.InfoContext(ctx, "unified entry created all three capabilities")

	const serviceName = "coordx-demo-service"
	inst := &coordx.ServiceInstance{
		ID:        "apollo-demo-1",
		ServiceID: serviceName,
		Addr:      "127.0.0.1",
		Port:      8080,
		Healthy:   true,
	}

	// 三个能力逐个调用，全部预期返回 ErrUnsupported
	if err := register.Register(ctx, serviceName, inst); err != nil {
		if errors.Is(err, coordx.ErrUnsupported) {
			logger.InfoContext(ctx, "Register 不被支持，按设计降级跳过", logx.Err(err))
		} else {
			logger.Fatal("Register 返回了非预期的错误", logx.Err(err))
		}
	} else {
		logger.Fatal("Register 意外成功：Apollo 没有注册原语，成功说明实现有误")
	}

	if _, err := discover.Discovery(ctx, serviceName); err != nil {
		if errors.Is(err, coordx.ErrUnsupported) {
			logger.InfoContext(ctx, "Discovery 不被支持，按设计降级跳过", logx.Err(err))
		} else {
			logger.Fatal("Discovery 返回了非预期的错误", logx.Err(err))
		}
	} else {
		logger.Fatal("Discovery 意外成功：Apollo 没有发现原语，成功说明实现有误")
	}

	if _, err := lease.Grant(ctx, 30*time.Second); err != nil {
		if errors.Is(err, coordx.ErrUnsupported) {
			logger.InfoContext(ctx, "Lease 不被支持，按设计降级跳过", logx.Err(err))
		} else {
			logger.Fatal("Lease 返回了非预期的错误", logx.Err(err))
		}
	} else {
		logger.Fatal("Lease 意外成功：Apollo 没有租约原语，成功说明实现有误")
	}

	fmt.Println("\n✅ All operations completed successfully.")
}
