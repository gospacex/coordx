# Apollo 驱动功能矩阵

> 代码依据：`apollo.go`、`config.go`、`register.go`（coordx/apollo）
> 更新日期：2026-09-09 | 驱动注册名：`apollo` | 客户端库：`github.com/ctripcorp/apollo-go` 或等价实现

## 1. 基础命令矩阵

### Register 接口

| 接口方法 | 状态 | 行为说明 |
|---|---|---|
| `Register(ctx, service, instance)` | ❌ 不支持 | Apollo 是配置中心，无服务注册/发现功能 |
| `Deregister(ctx, service, instance)` | ❌ 不支持 | — |
| `HealthCheck()` | ❌ 不支持 | — |
| `Close()` | ✅ 支持 | 关闭 Apollo 客户端 |

### Discover 接口

| 接口方法 | 状态 | 行为说明 |
|---|---|---|
| `Discovery(ctx, service)` | ❌ 不支持 | Apollo 无服务发现能力 |
| `Watch(ctx, service, callback)` | ✅ 支持 | Apollo 配置 Long Polling 监听变更 |
| `HealthCheck()` | ⚠️ 部分 | 检查 Apollo config/service 服务端连通性 |
| `Close()` | ✅ 支持 | — |

### Lease 接口

| 接口方法 | 状态 | 行为说明 |
|---|---|---|
| `Grant(ctx, ttl)` | ❌ 不支持 | Apollo 无 Lease 原语 |
| `Renew(ctx, sessionID)` | ❌ 不支持 | — |
| `Revoke(ctx, sessionID)` | ❌ 不支持 | — |
| `HealthCheck()` | — | — |
| `Close()` | — | — |

### OTel 追踪属性

| 属性 | 值 |
|---|---|
| span name | `coord.apollo.{COMMAND}` |
| `db.system` | `apollo` |

## 2. 部署拓扑矩阵

| 模式 | 状态 | 说明 |
|---|---|---|
| Apollo Config Service | ✅ 支持 | `cfg.Addr` 指向 Config Service |
| Apollo Admin Service | ⚠️ 部分 | 管理接口，驱动主要使用 Config Service |
| 多 Environment | ✅ 支持 | Apollo 原生 Environment（DEV/FW/PRO 等） |
| 多 Cluster | ✅ 支持 | Apollo 原生 Cluster 概念（数据中心） |
| TLS | ⚠️ 部分 | 依赖 Apollo 服务端配置 |
| Auth 认证 | ⚠️ 部分 | Apollo Access Key 机制，驱动接线程度有限 |

## 3. 配置字段生效状态（coordx.Config）

| 字段 | 状态 | 说明 |
|---|---|---|
| `Addr` | ✅ 生效 | Apollo Config Service 地址（如 `http://127.0.0.1:8080`） |
| `Cluster` | ✅ 生效 | Apollo Cluster 名称 |
| `Secret` | ⚠️ 部分 | 映射为 Apollo Access Key |
| `TLSEnabled` | ⚠️ 部分 | 接线但依赖服务端配置 |
| `Name` / `Driver` | ✅ 生效 | — |

## 4. 扩展命令矩阵

| 命令 | 状态 | 说明 |
|---|---|---|
| 多 Namespace 监听 | ✅ 支持 | Apollo 原生多 Namespace |
| 灰度发布 | ⚠️ 部分 | Apollo 支持灰度，但驱动未封装灰度发布接口 |
| 发布历史 | ⚠️ 部分 | Apollo 有发布历史，驱动未封装查询接口 |
| 分支发布 | ❌ 不支持 | — |

## 5. 企业级业务场景矩阵

| 业务场景 | 支持状态 | 实现方式与约束 |
|---|---|---|
| 配置中心 | ✅ 完整支持 | Long Polling 实时监听，Apollo 核心能力 |
| 服务注册中心 | ❌ 不支持 | Apollo 无服务注册/发现能力 |
| 服务发现 | ❌ 不支持 | — |
| 分布式锁 | ❌ 不支持 | 无 Lease 原语 |
| 多环境配置隔离 | ✅ 完整支持 | Environment + Cluster + Namespace 三维隔离 |
| 灰度配置 | ⚠️ 部分 | Apollo 支持但驱动未封装 |
| 健康探针 | ⚠️ 部分 | Config Service 连通性检查 |
| 链路追踪 | ✅ 支持 | `coord.apollo.*` span |

## 6. 已知限制与风险

1. **Register/Discover 完全不支持**：Apollo 是纯配置中心，不能作为服务注册/发现中间件使用；需要选择 etcd/nacos/consul 驱动替代。
2. **Lease 接口不支持**：无分布式锁能力。
3. **HealthCheck 能力有限**：仅检查 Config Service 连通性，不感知业务实例健康状态。
4. **Access Key 安全**：Apollo Access Key 用于管理接口认证，客户端配置需保护。

## 7. 快速接入示例

```go
import (
    "github.com/gospacex/coordx"
    _ "github.com/gospacex/coordx/apollo" // 注册驱动
)

discover, _ := coordx.NewDiscover(&coordx.Config{
    Name: "default", Driver: "apollo",
    Addr: "http://127.0.0.1:8080", // Apollo Config Service
    Cluster: "default",
})
// Apollo 无 Register/Lease 实现，这两个工厂函数将返回错误
```
