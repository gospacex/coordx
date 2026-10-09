// coordx/k8s_configmap/examples: K8s ConfigMap 服务注册示例
//
// ⚠️ 前置条件：需要一个可用的 K8s 集群与 kubeconfig。驱动会解析 config.yaml
// 里的 addr 作为 kubeconfig 路径；留空时回落到 ~/.kube/config。
// 没有集群时 NewRegister 会在建 client 阶段失败，示例直接退出（不会假装成功）。
//
// 能力边界（本示例同时演示）：
//   - Register / Deregister：真实实现（写/删 ConfigMap）
//   - Discovery / Watch：未实现 → 返回 coordx.ErrUnsupported
//   - Lease：ConfigMap 没有租约原语 → 返回 coordx.ErrUnsupported
//
// 运行：
//
//	cd coordx/k8s_configmap/examples && go run .
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/gospacex/configx"
	"github.com/gospacex/coordx"
	_ "github.com/gospacex/coordx/k8s_configmap"
	"github.com/gospacex/logx"
	_ "github.com/gospacex/logx/zap"
)

func main() {
	ctx := context.Background()

	if err := logx.ReadConfig(); err != nil {
		log.Printf("logx.ReadConfig fallback: %v", err)
	}
	logger := logx.GetGlobal()
	logger.InfoContext(ctx, "coordx k8s_configmap example started", logx.String("driver", "k8s_configmap"))

	register, discover, lease, err := coordx.ReadAndCreate(configx.ConfigOptions{
		ConfigFile: "config.yaml",
		ConfigType: "yaml",
	})
	if err != nil {
		logger.Fatal("create coordx client failed; 需要可用的 K8s 集群与 kubeconfig", logx.Err(err))
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

	if err := register.HealthCheck(); err != nil {
		logger.Fatal("register health check failed（连不上集群？）", logx.Err(err))
	}
	logger.InfoContext(ctx, "register health check passed")

	const serviceName = "coordx-demo-service"
	inst := &coordx.ServiceInstance{
		ID:        "k8s-demo-1",
		ServiceID: serviceName,
		Addr:      "127.0.0.1",
		Port:      8080,
		Healthy:   true,
	}

	// Register：在 namespace(默认 default) 下创建同名 ConfigMap
	if err := register.Register(ctx, serviceName, inst); err != nil {
		logger.Fatal("register failed", logx.Err(err))
	}
	logger.InfoContext(ctx, "service registered as configmap", logx.String("service", serviceName))

	// Deregister：删除该 ConfigMap
	if err := register.Deregister(ctx, serviceName, inst); err != nil {
		logger.ErrorContext(ctx, "deregister failed", logx.Err(err))
	} else {
		logger.InfoContext(ctx, "service deregistered")
	}

	// Discovery / Lease 未实现：用 errors.Is 判定降级，不当致命错误
	if _, err := discover.Discovery(ctx, serviceName); err != nil {
		if errors.Is(err, coordx.ErrUnsupported) {
			logger.InfoContext(ctx, "Discovery 未实现，按设计降级跳过", logx.Err(err))
		} else {
			logger.Fatal("Discovery 返回了非预期的错误", logx.Err(err))
		}
	}
	if _, err := lease.Grant(ctx, 30*time.Second); err != nil {
		if errors.Is(err, coordx.ErrUnsupported) {
			logger.InfoContext(ctx, "Lease 不被支持，按设计降级跳过", logx.Err(err))
		} else {
			logger.Fatal("Lease 返回了非预期的错误", logx.Err(err))
		}
	}

	fmt.Println("\n✅ All operations completed successfully.")
}
