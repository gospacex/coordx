# Nacos 驱动功能矩阵

> 代码依据：`nacos.go`、`config.go`、`register.go`（coordx/nacos）
> 更新日期：2026-09-09 | 驱动注册名：`nacos` | 客户端库：`github.com/nacos-group/nacos-sdk-go/v2`

## 1. 基础命令矩阵

### Register 接口

| 接口方法 | 底层实现 | 状态 | 行为说明 |
|---|---|---|---|
| `Register(ctx, service, instance)` | `namingClient.RegisterInstance()` | ✅ 支持 | 注册到 Nacos 服务列表，带实例权重/健康状态 |
| `Deregister(ctx, service, instance)` | `namingClient.DeregisterInstance()` | ✅ 支持 | 注销实例 |
| `HealthCheck()` | `namingClient.SelectAllInstances()` | ✅ 支持 | — |
| `Close()` | 客户端关闭 | ✅ 支持 | — |

### Discover 接口

| 接口方法 | 底层实现 | 状态 | 行为说明 |
|---|---|---|---|
| `Discovery(ctx, service)` | `namingClient.SelectInstances()` | ✅ 支持 | 获取指定服务的实例列表 |
| `Watch(ctx, service, callback)` | `namingClient.Subscribe()` | ✅ 支持 | 监听服务实例变更回调 |
| `HealthCheck()` | — | ✅ 支持 | — |
| `Close()` | — | ✅ 支持 | — |

### Lease 接口

| 接口方法 | 状态 | 行为说明 |
|---|---|---|
| `Grant(ctx, ttl)` | ❌ 不支持 | Nacos 无 Lease 原语，通过 Ephemeral Instance 的心跳保活模拟 |
| `Renew(ctx, sessionID)` | ❌ 不支持 | — |
| `Revoke(ctx, sessionID)` | ❌ 不支持 | — |
| `HealthCheck()` | — | — |
| `Close()` | — | — |

### OTel 追踪属性

| 属性 | 值 |
|---|---|
| span name | `coord.nacos.{COMMAND}` |
| `db.system` | `nacos` |

## 2. 部署拓扑矩阵

| 模式 | 状态 | 说明 |
|---|---|---|
| 单机 Nacos | ✅ 支持 | `cfg.Addr` 单地址 |
| Nacos Cluster | ✅ 支持 | `cfg.Cluster` 多地址 |
| Namespaces | ✅ 支持 | `cfg.Namespace` 多租户隔离（Nacos 原生 tenant 概念） |
| TLS | ⚠️ 部分 | Nacos 2.x gRPC 支持 TLS，但驱动接线程度有限 |
| Auth 认证 | ✅ 支持 | `cfg.Username`/`cfg.Secret` 传递 |
| 连接重试 | ✅ 支持 | Nacos 客户端内置重连 |

## 3. 配置字段生效状态（coordx.Config）

| 字段 | 状态 | 说明 |
|---|---|---|
| `Addr` | ✅ 生效 | Nacos 服务器地址 |
| `Cluster` | ✅ 生效 | 集群多节点 |
| `Secret` | ✅ 生效 | 认证密码 |
| `TLSEnabled` | ⚠️ 部分 | 接线但依赖 Nacos 服务端 TLS 配置 |
| `Name` / `Driver` | ✅ 生效 | — |

### 驱动自有配置（nacos/config.go）

Nacos 特有：`Namespace`（租户隔离）、`Group`（服务分组）、`Weight`（实例权重）、`HealthyOnly`（仅返回健康实例）。

## 4. 扩展命令矩阵

| 命令 | 状态 | 说明 |
|---|---|---|
| 服务分组 | ✅ 支持 | Nacos 原生 Group 概念 |
| 实例权重 | ✅ 支持 | 动态权重调整 |
| 实例健康标记 | ✅ 支持 | 瞬时/持久实例 |
| 配置监听 | ✅ 支持 | ConfigService 原生 Watch |

## 5. 企业级业务场景矩阵

| 业务场景 | 支持状态 | 实现方式与约束 |
|---|---|---|
| 服务注册中心 | ✅ 完整支持 | 原生服务管理 |
| 服务发现 | ✅ 完整支持 | Subscribe 实时感知 |
| 配置中心 | ✅ 完整支持 | ConfigService Watch |
| 分布式锁 | ❌ 不支持 | Lease 接口未实现 |
| Leader Election | ⚠️ 可间接支持 | 通过临时实例竞争注册实现，未封装 |
| 多租户隔离 | ✅ 支持 | Namespace 原生隔离 |
| 服务分组管理 | ✅ 支持 | Nacos 原生 Group |
| 健康探针 | ✅ 支持 | SelectAllInstances 检查 |
| 链路追踪 | ✅ 支持 | `coord.nacos.*` span |

## 6. 已知限制与风险

1. **Lease 接口不支持**：Nacos 无原生 Lease 原语，分布式锁场景需改用 etcd/Consul 驱动。
2. **Namespace 隔离边界**：不同 Namespace 的服务不可互相发现，跨租户服务发现需额外处理。
3. **临时实例心跳**：Ephemeral 实例依赖客户端心跳保活，进程崩溃后 Nacos 延迟剔除（默认 10s 检测）。
4. **配置中心与注册中心分离**：Nacos 两套 API（naming/config），驱动统一封装但内部为两个独立客户端。

## 7. 快速接入示例

```go
import (
    "github.com/gospacex/coordx"
    _ "github.com/gospacex/coordx/nacos" // 注册驱动
)

register, _ := coordx.New(&coordx.Config{
    Name: "default", Driver: "nacos",
    Addr: "127.0.0.1:8848", Secret: "nacos123456",
})
discover, _ := coordx.NewDiscover(&coordx.Config{
    Name: "default", Driver: "nacos",
    Addr: "127.0.0.1:8848", Secret: "nacos123456",
})
```
