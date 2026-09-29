# Kubernetes ConfigMap 驱动功能矩阵

> 代码依据：`k8s_configmap.go`、`config.go`、`register.go`（coordx/k8s_configmap）
> 更新日期：2026-09-09 | 驱动注册名：`k8s_configmap` | 客户端库：`k8s.io/client-go`

## 1. 基础命令矩阵

### Register 接口

| 接口方法 | 状态 | 行为说明 |
|---|---|---|
| `Register(ctx, service, instance)` | ❌ 不支持 | K8s ConfigMap 不是服务注册中心 |
| `Deregister(ctx, service, instance)` | ❌ 不支持 | — |
| `HealthCheck()` | ❌ 不支持 | — |
| `Close()` | ✅ 支持 | 停止 informer，关闭 K8s 客户端 |

### Discover 接口

| 接口方法 | 状态 | 行为说明 |
|---|---|---|
| `Discovery(ctx, service)` | ⚠️ 部分 | 通过读取 ConfigMap 数据模拟服务发现；需业务侧约定 ConfigMap 命名规范 |
| `Watch(ctx, service, callback)` | ✅ 支持 | K8s Informer 原生事件监听，ConfigMap 变更实时回调 |
| `HealthCheck()` | ✅ 支持 | `k8sClient.Discovery().ServerVersion()` 检查集群连通 |
| `Close()` | ✅ 支持 | 停止 informer |

### Lease 接口

| 接口方法 | 状态 | 行为说明 |
|---|---|---|
| `Grant(ctx, ttl)` | ❌ 不支持 | K8s 无 Lease 原语（Lease CRD 未封装） |
| `Renew(ctx, sessionID)` | ❌ 不支持 | — |
| `Revoke(ctx, sessionID)` | ❌ 不支持 | — |
| `HealthCheck()` | — | — |
| `Close()` | — | — |

### OTel 追踪属性

| 属性 | 值 |
|---|---|
| span name | `coord.k8s_configmap.{COMMAND}` |
| `db.system` | `kubernetes` |

## 2. 部署拓扑矩阵

| 模式 | 状态 | 说明 |
|---|---|---|
| K8s Cluster 内部 | ✅ 支持 | 驱动运行于 K8s Pod 内，通过 in-cluster config 连接 API Server |
| K8s Cluster 外部 | ✅ 支持 | 通过 kubeconfig 文件连接（`cfg.Addr` 指向 API Server） |
| RBAC 权限 | ✅ 支持 | 需 `get/list/watch/configmaps` 权限 |
| 多 Namespace | ⚠️ 部分 | 驱动固定 Namespace，跨 Namespace 需多实例 |
| 高可用 | ⚠️ 部分 | 依赖 K8s API Server HA，驱动自身无 HA 机制 |

## 3. 配置字段生效状态（coordx.Config）

| 字段 | 状态 | 说明 |
|---|---|---|
| `Addr` | ✅ 生效 | API Server 地址（外联模式）；in-cluster 模式下忽略 |
| `Cluster` | ⚠️ 部分 | 映射为 K8s Namespace |
| `Secret` | ⚠️ 部分 | K8s Token/Password（服务账户 token 自动挂载） |
| `TLSEnabled` | ✅ 生效 | — |
| `Name` / `Driver` | ✅ 生效 | — |

## 4. 扩展命令矩阵

| 命令 | 状态 | 说明 |
|---|---|---|
| Secret 读取 | ❌ 不支持 | 仅 ConfigMap，Secret 需额外封装 |
| Label Selector | ✅ 支持 | Informer 可用 label selector 过滤 |
| Field Selector | ✅ 支持 | 按 name/namespace 精确查找 |
| 多 ConfigMap 聚合 | ⚠️ 部分 | 需业务侧监听多个 ConfigMap 后聚合 |

## 5. 企业级业务场景矩阵

| 业务场景 | 支持状态 | 实现方式与约束 |
|---|---|---|
| K8s 内配置中心 | ✅ 优势场景 | ConfigMap 变更实时感知，K8s 原生能力 |
| 服务注册中心 | ❌ 不支持 | ConfigMap 非服务发现机制 |
| 服务发现 | ⚠️ 部分 | 通过 ConfigMap 约定模拟，非原生 |
| 分布式锁 | ❌ 不支持 | 无 Lease 原语 |
| Leader Election | ⚠️ 可间接支持 | 通过 ConfigMap 的 annotation/ownerReference 竞争，未封装 |
| 多租户隔离 | ✅ 支持 | Namespace 原生隔离 |
| 健康探针 | ✅ 支持 | API Server version 检查 |
| 链路追踪 | ✅ 支持 | `coord.k8s_configmap.*` span |

## 6. 已知限制与风险

1. **Register 完全不支持**：K8s ConfigMap 不是服务注册中心，不可作为 Register 实现。
2. **Lease 完全不支持**：无分布式锁能力，需改用 etcd 驱动。
3. **Discovery 为模拟实现**：通过 ConfigMap 数据约定模拟服务发现，缺乏标准服务网格语义。
4. **Namespace 固定**：驱动单实例绑定单一 Namespace，跨 Namespace 发现需多实例。
5. **RBAC 依赖**：Pod 需具备 ConfigMap get/list/watch 权限，否则初始化失败。

## 7. 快速接入示例

```go
import (
    "github.com/gospacex/coordx"
    _ "github.com/gospacex/coordx/k8s_configmap" // 注册驱动
)

discover, _ := coordx.NewDiscover(&coordx.Config{
    Name: "default", Driver: "k8s_configmap",
    Addr: "https://kubernetes.default.svc:443",
    Cluster: "default", // K8s Namespace
})
// Register/Lease 在此驱动上不可用
```
