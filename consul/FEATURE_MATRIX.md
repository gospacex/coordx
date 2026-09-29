# Consul 驱动功能矩阵

> 代码依据：`consul.go`、`config.go`、`register.go`（coordx/consul）
> 更新日期：2026-09-09 | 驱动注册名：`consul` | 客户端库：`github.com/hashicorp/consul/api`

## 1. 基础命令矩阵

### Register 接口

| 接口方法 | 底层实现 | 状态 | 行为说明 |
|---|---|---|---|
| `Register(ctx, service, instance)` | `client.Catalog.Register()` + `client.Agent().ServiceRegister()` | ✅ 支持 | 注册服务实例，带健康检查脚本/HTTP 端点 |
| `Deregister(ctx, service, instance)` | `client.Catalog.Deregister()` | ✅ 支持 | — |
| `HealthCheck()` | `client.Health().Service()` | ✅ 支持 | 查询健康实例 |
| `Close()` | 客户端关闭 | ✅ 支持 | — |

### Discover 接口

| 接口方法 | 底层实现 | 状态 | 行为说明 |
|---|---|---|---|
| `Discovery(ctx, service)` | `client.Health().Service()` | ✅ 支持 | 返回健康实例列表（默认只返回 healthy） |
| `Watch(ctx, service, callback)` | `client.Health().Service()` + blocking query | ✅ 支持 | Consular blocking query 实现长轮询监听 |
| `HealthCheck()` | — | ✅ 支持 | — |
| `Close()` | — | ✅ 支持 | — |

### Lease 接口

| 接口方法 | 状态 | 行为说明 |
|---|---|---|
| `Grant(ctx, ttl)` | ❌ 不支持 | Consul 无 Lease 原语，通过 Session 模拟 |
| `Renew(ctx, sessionID)` | ❌ 不支持 | — |
| `Revoke(ctx, sessionID)` | ❌ 不支持 | — |
| `HealthCheck()` | — | — |
| `Close()` | — | — |

### OTel 追踪属性

| 属性 | 值 |
|---|---|
| span name | `coord.consul.{COMMAND}` |
| `db.system` | `consul` |

## 2. 部署拓扑矩阵

| 模式 | 状态 | 说明 |
|---|---|---|
| 单机 Consul | ✅ 支持 | `cfg.Addr` 单地址 |
| Consul Cluster | ✅ 支持 | `cfg.Cluster` 多地址，客户端自动负载均衡 |
| TLS/mTLS | ✅ 支持 | `cfg.TLSEnabled` → Consul TLS config |
| Auth 认证 | ✅ 支持 | `cfg.Secret` 传递 ACL token |
| 多 DC 部署 | ✅ 支持 | Consul 原生多 DC，驱动支持 `cfg.Cluster` 跨 DC 地址 |
| 连接重试 | ✅ 支持 | Consul 客户端内置重连 |

## 3. 配置字段生效状态（coordx.Config）

| 字段 | 状态 | 说明 |
|---|---|---|
| `Addr` | ✅ 生效 | 单节点地址 |
| `Cluster` | ✅ 生效 | 多节点地址（逗号分隔） |
| `Secret` | ✅ 生效 | ACL token |
| `TLSEnabled` | ✅ 生效 | — |
| `Name` / `Driver` | ✅ 生效 | — |

## 4. 扩展命令矩阵

| 命令 | 状态 | 说明 |
|---|---|---|
| Session 管理 | ⚠️ 部分 | Consul 原生 Session，可用于分布式锁模拟，但 Lease 接口未封装 |
| 健康检查脚本 | ✅ 支持 | Register 时可附加 HTTP/TTL/DNS 健康检查 |
| 服务元数据 Tag | ✅ 支持 | 实例标签携带版本/环境信息 |
| Key-Value 存储 | ⚠️ 部分 | Consul KV 可用，但与注册中心分属不同 API，未统一封装 |

## 5. 企业级业务场景矩阵

| 业务场景 | 支持状态 | 实现方式与约束 |
|---|---|---|
| 服务注册中心 | ✅ 完整支持 | 原生服务管理 + 健康检查 |
| 服务发现 | ✅ 完整支持 | blocking query 实时感知 |
| 配置中心 | ⚠️ 部分 | Consul KV 可用但未封装为协调接口 |
| 分布式锁 | ⚠️ 可间接支持 | Session + Key 竞争实现，未封装 Lease 接口 |
| Leader Election | ⚠️ 可间接支持 | Session 竞争注册实现 |
| 多 DC 服务发现 | ✅ 支持 | Consul 原生多 DC 路由 |
| 健康探针 | ✅ 支持 | Health().Service() |
| 链路追踪 | ✅ 支持 | `coord.consul.*` span |

## 6. 已知限制与风险

1. **Lease 接口不支持**：Consul 无 Lease 原语，Session 模拟的锁与 Lease 语义有差异。
2. **Health Check 默认 healthy only**：`Discovery` 默认只返回 healthy 实例， unhealthy 实例需显式传 `health.ChecksIncluded()`。
3. **blocking query 超时**：Watch 长轮询默认 5min 超时，需业务侧处理超时重发。
4. **服务与健康检查耦合**：注册时健康检查脚本/端点需额外配置，驱动未封装健康检查定义。

## 7. 快速接入示例

```go
import (
    "github.com/gospacex/coordx"
    _ "github.com/gospacex/coordx/consul" // 注册驱动
)

register, _ := coordx.New(&coordx.Config{
    Name: "default", Driver: "consul",
    Addr: "127.0.0.1:8500", Secret: "consul123456",
    TLSEnabled: false,
})
discover, _ := coordx.NewDiscover(&coordx.Config{
    Name: "default", Driver: "consul",
    Addr: "127.0.0.1:8500", Secret: "consul123456",
})
```
