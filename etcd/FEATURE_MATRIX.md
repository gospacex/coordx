# etcd 驱动功能矩阵

> 代码依据：`etcd.go`、`config.go`、`register.go`（coordx/etcd）
> 更新日期：2026-09-09 | 驱动注册名：`etcd` | 客户端库：`go.etcd.io/etcd/client/v3`

## 1. 基础命令矩阵

### Register 接口

| 接口方法 | 底层实现 | 状态 | 行为说明 |
|---|---|---|---|
| `Register(ctx, service, instance)` | `client.KV.Put(ctx, key, value)` + Lease KeepAlive | ✅ 支持 | 通过 Lease 绑定实例注册，租约到期自动注销 |
| `Deregister(ctx, service, instance)` | `client.KV.Delete(ctx, key)` | ✅ 支持 | — |
| `HealthCheck()` | `client.MemberList(ctx)` | ✅ 支持 | 检查集群可达性 |
| `Close()` | 客户端关闭 | ✅ 支持 | — |

### Discover 接口

| 接口方法 | 底层实现 | 状态 | 行为说明 |
|---|---|---|---|
| `Discovery(ctx, service)` | `client.KV.Get(ctx, prefix, withPrefix)` | ✅ 支持 | 前缀扫描返回所有健康实例 |
| `Watch(ctx, service, callback)` | `client.Watch(ctx, prefix)` | ✅ 支持 | 实时监听服务注册/注销事件 |
| `HealthCheck()` | `client.MemberList(ctx)` | ✅ 支持 | — |
| `Close()` | 关闭 watch stream | ✅ 支持 | — |

### Lease 接口

| 接口方法 | 底层实现 | 状态 | 行为说明 |
|---|---|---|---|
| `Grant(ctx, ttl)` | `client.Lease.Grant(ctx, ttl)` | ✅ 支持 | 创建租约，返回 sessionID（lease ID） |
| `Renew(ctx, sessionID)` | `client.Lease.KeepAliveOnce(ctx, leaseID)` | ✅ 支持 | 续期一次；需业务侧循环调用 |
| `Revoke(ctx, sessionID)` | `client.Lease.Revoke(ctx, leaseID)` | ✅ 支持 | 主动销毁租约 |
| `HealthCheck()` | — | ✅ 支持 | — |
| `Close()` | — | ✅ 支持 | — |

### OTel 追踪属性

| 属性 | 值 |
|---|---|
| span name | `coord.etcd.{COMMAND}` |
| `db.system` | `etcd` |

## 2. 部署拓扑矩阵

| 模式 | 状态 | 说明 |
|---|---|---|
| 单机 etcd | ✅ 支持 | `cfg.Addr` 单地址 |
| 多节点 Cluster | ✅ 支持 | `cfg.Addrs` 多地址，客户端自动负载均衡 |
| TLS/mTLS | ✅ 支持 | `cfg.TLSEnabled` → ETCD mTLS config |
| Auth 认证 | ✅ 支持 | `cfg.Username`/`cfg.Password` |
| 连接重试 | ✅ 支持 | etcd 客户端内置自动重连 |
| 多 DC 部署 | ⚠️ 部分 | etcd 支持多 DC，但驱动未封装 DC 感知路由 |

## 3. 配置字段生效状态（coordx.Config）

| 字段 | 状态 | 说明 |
|---|---|---|
| `Addr` | ✅ 生效 | 单节点地址 |
| `Cluster` | ✅ 生效 | 逗号分隔多节点地址 |
| `Secret` | ✅ 生效 | 密码（Basic Auth） |
| `TLSEnabled` | ✅ 生效 | — |
| `Name` / `Driver` | ✅ 生效 | — |

## 4. 扩展命令矩阵

| 命令 | 状态 | 说明 |
|---|---|---|
| 服务标签/元数据 | ✅ 支持 | Register 时通过 value 携带 JSON 元数据 |
|  TTL 自定义 | ✅ 支持 | Lease.Grant 传入精确 TTL |
| 批量注册 | ⚠️ 部分 | 需业务侧循环调用，无原子批量 API |
| 健康检查间隔 | ⚠️ 部分 | 依赖 Lease TTL，非独立配置 |

## 5. 企业级业务场景矩阵

| 业务场景 | 支持状态 | 实现方式与约束 |
|---|---|---|
| 服务注册中心 | ✅ 完整支持 | Register/Deregister + Watch 全覆盖 |
| 服务发现 | ✅ 完整支持 | Discovery + Watch 实时感知变更 |
| 分布式锁 | ✅ 完整支持 | Lease Grant/Renew/Revoke 原生支持 |
| 配置中心 | ✅ 完整支持 | Watch 实时监听配置变更 |
|  Leader Election | ✅ 完整支持 | Lease + 竞争注册天然支持 |
| 多租户隔离 | ✅ 支持 | Prefix 隔离命名空间 |
| 跨 DC 服务发现 | ⚠️ 部分 | etcd 支持，但驱动未封装 DC 路由 |
| 健康探针 | ✅ 支持 | MemberList 检查 |
| 链路追踪 | ✅ 支持 | `coord.etcd.*` span |

## 6. 已知限制与风险

1. **Renew 非自动**：`Lease.Renew` 需业务侧循环调用 KeepAlive；驱动未封装自动续期 goroutine。
2. **Watch 断连需重连**：etcd Watch 在长连接中断后需重新发起，驱动未封装自动重连。
3. **Discovery 非实时**：`Discovery` 快照扫描，新注册实例需 Watch 感知。
4. **值大小限制**：单值最大 1.5MB，实例元数据过大时需拆分存储。
5. **集群依赖**：etcd 本身是强一致集群，部署运维复杂度高于 Nacos/Consul。

## 7. 快速接入示例

```go
import (
    "github.com/gospacex/coordx"
    _ "github.com/gospacex/coordx/etcd" // 注册驱动
)

register, _ := coordx.New(&coordx.Config{
    Name: "default", Driver: "etcd",
    Addr: "127.0.0.1:2379", Secret: "etcd123456",
    TLSEnabled: false,
})
discover, _ := coordx.NewDiscover(&coordx.Config{
    Name: "default", Driver: "etcd",
    Addr: "127.0.0.1:2379", Secret: "etcd123456",
})
lease, _ := coordx.NewLease(&coordx.Config{
    Name: "default", Driver: "etcd",
    Addr: "127.0.0.1:2379", Secret: "etcd123456",
})
```
