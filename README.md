<div align="center">

# coordx

**HubX 服务注册与发现 / 分布式协调统一 SDK**

A unified Go SDK for service registration, discovery and lease coordination —
one set of `Register` / `Discover` / `Lease` interfaces over etcd, Consul, Nacos, Apollo and Kubernetes ConfigMap.

[![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)](https://go.dev/)
![License](https://img.shields.io/badge/License-MIT-green.svg)
[![Release](https://img.shields.io/badge/release-v0.0.1-blue.svg)](https://github.com/gospacex/coordx/tags)

</div>

---

## 目录

- [1. 定位与设计边界](#1-定位与设计边界)
- [2. 核心特性](#2-核心特性)
- [3. 架构](#3-架构)
- [4. 快速开始](#4-快速开始)
- [5. 配置参考](#5-配置参考)
- [6. API 参考](#6-api-参考)
- [7. 支持的后端矩阵](#7-支持的后端矩阵)
- [8. 可观测性](#8-可观测性)
- [9. 示例与文档](#9-示例与文档)
- [10. 测试与质量](#10-测试与质量)
- [11. 版本与兼容性](#11-版本与兼容性)
- [12. 许可证](#12-许可证)

---

## 1. 定位与设计边界

### 解决什么问题

coordx 把服务协调拆成三个正交原语，并为每个原语提供统一的 Go 接口：

- **Register（注册）**：把服务实例写入注册中心，支持注销与健康检查
- **Discover（发现）**：拉取服务实例列表，并监听实例变更
- **Lease（租约）**：授予 / 续期 / 撤销带 TTL 的分布式会话，用于锁与选主等场景

业务代码只依赖 `coordx.Register` / `coordx.Discover` / `coordx.Lease` 三个接口，切换后端只需替换 `Config.Driver` 与一行空导入。

### 不解决什么问题

- **不做 RPC / 服务网格**：只负责实例元数据的登记与读取，不代理流量
- **不提供配置中心客户端**：配置读取委托给 `configx`，coordx 只定义 `coordx` 配置节的结构
- **不内置分布式锁实现**：`Lease` 只是接口原语；当前各驱动的可用性见 [第 7 节](#7-支持的后端矩阵)
- **不负责 OTel Provider 装配**：埋点开关由 `traceflag` 收敛，exporter 由宿主应用装配

### 与同系列 SDK 的关系

| 组件 | 关系 |
|---|---|
| `configx` | 配置句柄来源。`coordx.ReadConfig` / `ReadAndCreate` 直接调用 `configx.ReadConfig`，读取 `coordx` 配置节 |
| `logx` | 结构化日志。各驱动通过 `logx.GetGlobal()` 输出带 ctx 字段的运行日志 |
| `traceflag` | 调用链开关。各驱动用 `traceflag.NewTracer(cfg.EnableTrace, ..., "coordx")` 创建惰性 tracer |
| OpenTelemetry | span 语义由 traceflag 承接，Provider/exporter 由宿主应用自行装配（见 [第 8 节](#8-可观测性)） |

---

## 2. 核心特性

- **三大原语统一接口**：`Register` / `Discover` / `Lease` 各自收敛为 4~5 个方法，换后端不改业务代码。
- **驱动自注册工厂**：每个驱动在 `init()` 中调用 `coordx.RegisterCreator` / `RegisterDiscoverCreator` / `RegisterLeaseCreator` 注册构造函数，业务侧空导入即可接入。
- **五个后端驱动，各自独立 `go.mod`**：`etcd` / `consul` / `nacos` / `apollo` / `k8s_configmap` 均为独立子模块，可按需引入，避免拉入全部客户端依赖。
- **统一配置入口**：`ReadAndCreate` 从 `configx` 的 `coordx` 配置节一次性构造 register / discover / lease 三个实例。
- **结构化日志**：全部操作走 `logx` 的 `InfoContext` / `ErrorContext`，携带 `service` / `inst_id` / `addr` 等字段。
- **可开关的调用链埋点**：`EnableTrace`（实例级）∧ 组件级 `coordx` ∧ 全局开关三者同时打开才产生 span，默认不产生。
- **连接兜底**：驱动均带默认地址与重试（如 etcd 连接最多重试 3 次、指数退避），本地 `docker compose` 可零配置起服务。

---

## 3. 架构

```
业务代码
   │  coordx.NewRegister / NewDiscover / NewLease / ReadAndCreate
   ▼
接口层  coordx/interface.go
   │  Register · Discover · Lease · ServiceInstance · Config
   │  工厂注册表 registerCreators / discoverCreators / leaseCreators
   ▼
配置层  coordx/config.go
   │  StandaloneConfig · ClusterConfig · CoordSection
   │  ReadConfig / ReadAndCreate / MustReadAndCreate → configx
   ▼
驱动层（独立 go.mod 子模块，init() 自注册）
   ├─ etcd           go.etcd.io/etcd/client/v3      Register ✅  Discover ⚠️  Lease ❌
   ├─ consul         github.com/hashicorp/consul/api Register ✅  Discover ⚠️  Lease ❌
   ├─ nacos          nacos-sdk-go/v2                Register ❌  Discover ✅  Lease ❌
   ├─ apollo         （无后端客户端，stub）           Register ❌  Discover ❌  Lease ❌
   └─ k8s_configmap  k8s.io/client-go               Register ✅  Discover ❌  Lease ❌
   ▼
横切能力   logx（结构化日志） · traceflag（OTel span 开关）
```

> 详细的逐能力矩阵与已知限制见 [第 7 节](#7-支持的后端矩阵)。

---

## 4. 快速开始

### 4.1 安装

根模块与驱动子模块是彼此独立的 Go module，需要同时引入：

```bash
go get github.com/gospacex/coordx@v0.0.1
go get github.com/gospacex/coordx/etcd@v0.0.1
```

> 其余驱动按需替换路径：`coordx/consul`、`coordx/nacos`、`coordx/apollo`、`coordx/k8s_configmap`。

### 4.2 最小可用示例

先起一个本地 etcd（或直接用仓库内 `docker compose up -d etcd`）：

```go
package main

import (
	"context"
	"log"

	"github.com/gospacex/coordx"
	_ "github.com/gospacex/coordx/etcd" // 空导入触发 init() 注册驱动
)

func main() {
	ctx := context.Background()

	reg, err := coordx.NewRegister(&coordx.Config{
		Driver: "etcd",
		Addr:   "localhost:2379",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer reg.Close()

	inst := &coordx.ServiceInstance{
		ID:       "demo-1",
		Addr:     "127.0.0.1",
		Port:     8080,
		Metadata: map[string]string{"version": "1.0.0"},
		Healthy:  true,
	}
	if err := reg.Register(ctx, "demo-service", inst); err != nil {
		log.Fatal(err)
	}
	if err := reg.Deregister(ctx, "demo-service", inst); err != nil {
		log.Fatal(err)
	}
}
```

发现侧同理（etcd / consul / nacos 已注册 Discover 工厂）：

```go
disc, err := coordx.NewDiscover(&coordx.Config{Driver: "etcd", Addr: "localhost:2379"})
if err != nil {
	log.Fatal(err)
}
defer disc.Close()

instances, err := disc.Discovery(ctx, "demo-service")
if err != nil {
	log.Fatal(err)
}
for _, in := range instances {
	log.Printf("%s -> %s:%d healthy=%v", in.ID, in.Addr, in.Port, in.Healthy)
}
```

### 4.3 进阶用法（配置文件方式）

`examples/etcd/config.yaml` 是真实的配置形态：

```yaml
coordx:
  standalone:
    - driver: etcd
      name: default
      addr: "localhost:2379"
  cluster:
    - driver: etcd
      name: cluster
      addr: "localhost:2379"
      cluster: "etcd-cluster-1"
      secret: ""
```

一次性构造三个实例（与 `examples/etcd/main.go` 一致）：

```go
import (
	"github.com/gospacex/configx"
	"github.com/gospacex/coordx"
	_ "github.com/gospacex/coordx/etcd"
)

register, discover, lease, err := coordx.ReadAndCreate(configx.ConfigOptions{
	ConfigFile: "config.yaml",
	ConfigType: "yaml",
})
if err != nil {
	log.Fatal(err)
}
defer register.Close()
defer discover.Close()
defer lease.Close()
```

**注意**：`ReadAndCreate` 会依次调用 `NewRegister` → `NewDiscover` → `NewLease`，任一工厂未注册即整体失败。
当前只有 `nacos` 同时注册了三个工厂（其可用性见 [7.2](#72-已知限制)），etcd / consul / apollo / k8s_configmap 会在
`NewLease` 处返回 `coordx: lease driver "..." not registered`。按需构造时请直接调用 `coordx.NewRegister` / `coordx.NewDiscover`。

---

## 5. 配置参考

### 5.1 全局配置节 `coordx`

由 `coordx.CoordSection` 承接（`config.go` 中的 yaml tag 逐一核对），`ReadAndCreate` 读取：

```yaml
coordx:
  # 单实例：本地 / 单机部署
  standalone:
    - driver: etcd          # 驱动名，对应工厂注册名
      name: default         # 实例名
      addr: "localhost:2379"
  # 集群：多节点部署
  cluster:
    - driver: nacos
      name: cluster
      addr: "localhost:8848"
      cluster: "cluster-1"        # 集群名
      secret: ""                  # 集群密钥
      namespace: "public"         # 命名空间（多租户隔离）
      watch_enabled: true         # 是否监听配置变更
```

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|---|---|---|---|---|
| `standalone[].driver` | string | 是 | — | 驱动名：`etcd` / `consul` / `nacos` / `apollo` / `k8s_configmap` |
| `standalone[].name` | string | 否 | `default` | 实例名（`FillDefaults` 兜底） |
| `standalone[].addr` | string | 否 | 各驱动默认地址 | 后端地址，见 5.4 |
| `cluster[].driver` | string | 是 | — | 同上 |
| `cluster[].name` | string | 否 | `default` | 实例名 |
| `cluster[].addr` | string | 否 | 各驱动默认地址 | 后端地址 |
| `cluster[].cluster` | string | 否 | 空 | 集群名（映射到 `coordx.Config.Cluster`） |
| `cluster[].secret` | string | 否 | 空 | 集群密钥（映射到 `coordx.Config.Secret`） |
| `cluster[].namespace` | string | 否 | 空 | 命名空间；**当前 `ReadAndCreate` 未读取该字段** |
| `cluster[].watch_enabled` | bool | 否 | `false` | **当前 `ReadAndCreate` 未读取该字段** |

`ReadAndCreate` 的选择规则（源码 `config.go`）：**优先取 `cluster` 数组第一个元素，为空时取 `standalone` 数组第一个元素**；
两者皆空则返回 `coordx: no coordx instance configured`。

### 5.2 `coordx.Config`（代码构造方式）

`coordx.Config` 无 yaml/json tag，仅通过代码赋值：

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `Driver` | string | 无 | 工厂注册名，未注册时 `NewXxx` 返回 `driver "..." not registered` |
| `Name` | string | `default` | 由 `FillDefaults()` 填充；当前各驱动构造函数未消费该字段 |
| `Addr` | string | 见 5.4 | 后端地址；k8s_configmap 中语义为 kubeconfig 路径 |
| `Cluster` | string | 空 | 集群 / 命名空间；k8s_configmap 中映射为 Namespace |
| `Secret` | string | 空 | 密钥；当前各驱动构造函数均未消费该字段 |
| `TLSEnabled` | bool | `false` | 仅 etcd 驱动消费（`tls.Config{InsecureSkipVerify: true}`） |
| `EnableTrace` | bool | `false` | 实例级调用链开关，与组件级 / 全局开关为 AND 关系 |

### 5.3 驱动侧配置结构（带 yaml / json tag）

各驱动子模块导出 `StandaloneConfig` / `ClusterConfig`，并提供 `ToStandaloneConfig(name)` / `ToClusterConfig(name)`
转换为 `*coordx.Config`（`Namespace` → `Config.Cluster`，`endpoints[0]` → `Config.Addr`）。
**这些结构体当前未被 `ReadAndCreate` 反序列化**，供宿主自行绑定 YAML 后转换使用。

| 驱动 | `StandaloneConfig` 键 | `ClusterConfig` 键 |
|---|---|---|
| etcd | `driver` `name` `addr` `ttl` `tls_enabled` `enable_trace` | `driver` `name` `endpoints` `username` `password` `ttl` `namespace` `tls_enabled` `enable_trace` |
| consul | `driver` `name` `addr` `token` `namespace` `tls_enabled` `enable_trace` | `driver` `name` `endpoints` `token` `namespace` `tls_enabled` `enable_trace` |
| nacos | `driver` `name` `addr` `username` `password` `namespace` `tls_enabled` `enable_trace` | `driver` `name` `endpoints` `username` `password` `namespace` `tls_enabled` `enable_trace` |
| apollo | `driver` `name` `addr` `namespace` `app_id` `enable_trace` | `driver` `name` `endpoints` `namespace` `app_id` `tls_enabled` `enable_trace` |
| k8s_configmap | `driver` `name` `addr` `namespace` `enable_trace` | `driver` `name` `addr` `endpoints` `namespace` `context` `tls_enabled` `enable_trace` |

> `ttl` 为 `time.Duration`（YAML 中可写 `30s`）；`endpoints` 数组字段仅在上述转换函数中被读取首个元素。

### 5.4 默认值与字段实际生效状态

构造阶段从源码可确认的默认值与消费情况：

| 驱动 | `Addr` 缺省值 | 实际消费的 `Config` 字段 | 其他硬编码默认值 |
|---|---|---|---|
| etcd | `http://localhost:2379` | `Addr` `TLSEnabled` `EnableTrace` | DialTimeout 5s；注册租约 TTL 30s；Register 路径连接重试 3 次（100ms/200ms 退避） |
| consul | `http://localhost:8500` | `Addr` `EnableTrace` | 健康检查 TTL 30s、Interval 10s、90s 后注销；`Discovery` 只返回 passing 实例 |
| nacos | `http` + `:8848` + `/nacos`（`Addr` 支持 `host:port` 或 `http://host:port`） | `Addr` `EnableTrace`（仅 Discover 构造 tracer） | NamingClient `TimeoutMs=5000` |
| apollo | `http://localhost:8070` | `Addr` `EnableTrace` | — |
| k8s_configmap | `~/.kube/config` | `Addr`（kubeconfig 路径）`Cluster`（Namespace）`EnableTrace` | Namespace 空则取 `default` |

> `Secret` / `Cluster`（etcd、consul、nacos 路径下）/ `ttl` / `username` / `password` / `token` 等字段虽在配置结构中定义，
> 但**当前驱动构造函数尚未接入客户端**，配置后不会生效。

---

## 6. API 参考

### 6.1 三大接口（`interface.go`）

```go
// ServiceInstance 服务实例
type ServiceInstance struct {
	ID        string
	ServiceID string
	Addr      string
	Port      int
	Metadata  map[string]string
	Healthy   bool
}

// Register 注册服务
type Register interface {
	Register(ctx context.Context, service string, instance *ServiceInstance) error
	Deregister(ctx context.Context, service string, instance *ServiceInstance) error
	HealthCheck() error
	Close() error
}

// Discover 服务发现
type Discover interface {
	Discovery(ctx context.Context, service string) ([]*ServiceInstance, error)
	Watch(ctx context.Context, service string, callback func([]*ServiceInstance)) error
	HealthCheck() error
	Close() error
}

// Lease 分布式协调租约
type Lease interface {
	Grant(ctx context.Context, ttl time.Duration) (string, error)
	Renew(ctx context.Context, sessionID string) error
	Revoke(ctx context.Context, sessionID string) error
	HealthCheck() error
	Close() error
}
```

### 6.2 工厂与配置函数

| 签名 | 说明 |
|---|---|
| `func NewRegister(cfg *Config) (Register, error)` | 按 `cfg.Driver` 查找注册中心工厂；未注册返回 `coordx: register driver "..." not registered` |
| `func NewDiscover(cfg *Config) (Discover, error)` | 按 `cfg.Driver` 查找发现器工厂；未注册返回 `coordx: discover driver "..." not registered` |
| `func NewLease(cfg *Config) (Lease, error)` | 按 `cfg.Driver` 查找租约工厂；未注册返回 `coordx: lease driver "..." not registered` |
| `func RegisterCreator(name string, creator func(*Config) (Register, error))` | 注册 Register 工厂（驱动 `init()` 调用） |
| `func RegisterDiscoverCreator(name string, creator func(*Config) (Discover, error))` | 注册 Discover 工厂 |
| `func RegisterLeaseCreator(name string, creator func(*Config) (Lease, error))` | 注册 Lease 工厂 |
| `func (c *Config) FillDefaults()` | `Name` 为空时填 `default`；三个 `NewXxx` 内部都会调用 |
| `func ReadConfig(opts ...configx.ConfigOptions) error` | 仅加载统一配置句柄 |
| `func ReadAndCreate(opts ...configx.ConfigOptions) (Register, Discover, Lease, error)` | 从 `coordx` 配置节构造三个实例 |
| `func MustReadAndCreate(opts ...configx.ConfigOptions) (Register, Discover, Lease)` | 同上，失败时打印错误并 `os.Exit(1)` |

### 6.3 驱动子模块导出符号

| 子模块（import path） | 包名 | 导出构造函数 | 导出配置类型 |
|---|---|---|---|
| `github.com/gospacex/coordx/etcd` | `etcd` | `NewRegister` `NewDiscover` | `StandaloneConfig` `ClusterConfig`（含 `ToStandaloneConfig` / `ToClusterConfig`） |
| `github.com/gospacex/coordx/consul` | `consul` | `NewRegister` `NewDiscover` | 同上 |
| `github.com/gospacex/coordx/nacos` | `nacos` | `NewRegister` `NewDiscover` `NewLease` | 同上 |
| `github.com/gospacex/coordx/apollo` | `apollo` | `NewRegister` | 同上 |
| `github.com/gospacex/coordx/k8s_configmap` | `k8sconfigmap` | `NewRegister` | 同上 |

> 注意：`k8s_configmap` 的 import path 含下划线，但 **package 名是 `k8sconfigmap`**。
> 直接调用子模块构造函数（而非通过工厂）时，返回的是未导出的具体类型 `*registerImpl` 等，只能按接口使用。

---

## 7. 支持的后端矩阵

### 7.1 能力矩阵

✅ = 已实现并调用后端 API；⚠️ = 已接线但存在明显缺口；❌ = 未注册工厂（`NewXxx` 报 not registered）或实现为 stub（调用返回错误 / 不可用）。

| 能力 | etcd | consul | nacos | apollo | k8s_configmap |
|---|---|---|---|---|---|
| `Register` 注册 | ✅ Put + 租约绑定 | ✅ Agent ServiceRegister | ❌ stub | ❌ stub | ✅ 创建 ConfigMap |
| `Deregister` 注销 | ✅ Delete | ✅ ServiceDeregister | ❌ stub | ❌ stub | ✅ 删除 ConfigMap |
| `Discovery` 拉取列表 | ⚠️ 未解码实例值 | ✅ Health().Service | ✅ SelectAllInstances | ❌ 无工厂 | ❌ 无工厂 |
| `Watch` 变更监听 | ⚠️ 有回调但数据同上 | ❌ 空实现（仅日志） | ✅ Subscribe 回调 | ❌ 无工厂 | ❌ 无工厂 |
| `Lease.Grant` 授予 | ❌ 无工厂 | ❌ 无工厂 | ❌ 构造即失败 | ❌ 无工厂 | ❌ 无工厂 |
| `Lease.Renew` 续期 | ❌ 无工厂 | ❌ 无工厂 | ❌ 构造即失败 | ❌ 无工厂 | ❌ 无工厂 |
| `Lease.Revoke` 撤销 | ❌ 无工厂 | ❌ 无工厂 | ❌ 构造即失败 | ❌ 无工厂 | ❌ 无工厂 |
| `HealthCheck()` | ✅ 恒返回 nil | ✅ 恒返回 nil | ✅ 恒返回 nil | ✅ 恒返回 nil | ✅ List ConfigMap 探活 |
| `Close()` | ✅ 关闭 etcd 客户端 | ✅ 恒返回 nil | ✅ 恒返回 nil | ✅ 恒返回 nil | ✅ 恒返回 nil |
| 注册心跳 / 保活 | ✅ 30s 租约 + KeepAlive | ✅ TTL 健康检查（30s/10s） | —（Nacos 客户端心跳，封装未落地） | — | — |
| OTel 埋点 | ✅ | ✅ | ⚠️ 仅 Discover 有 tracer | ✅ | ✅ |
| 独立 `go.mod` | ✅ | ✅ | ✅ | ✅ | ✅ |
| 示例目录 | ✅ `examples/etcd` | ❌ | ❌ | ❌ | ❌ |

**结论**：目前**只有 etcd 与 consul 提供可用的 Register，只有 nacos 提供可用的 Discover（含 Watch）**；
**没有任何一个驱动提供了可用的 `Lease` 实现**——`Lease` 原语在当前代码中尚无可用后端。

### 7.2 已知限制

1. **`Lease` 无可用后端**：只有 `nacos` 注册了 Lease 工厂，但 `NewLease` 内部硬编码的 `ServerConfig` 缺少 `IpAddr`，
   构造即返回 `coordx/nacos: create config client: [client.SetServerConfig] configs[0] is invalid`；
   etcd / consul / apollo / k8s_configmap 均未注册 Lease 工厂。
2. **`nacos` Register / Deregister 为 stub**：方法体内直接返回 `nacos: stub`；且 `registerImpl.tracer` 未初始化，
   实际调用会触发空指针 panic。`nacos` 当前只能用于**发现**。
3. **`apollo` Register / Deregister 为 stub**：返回 `apollo: not implemented in stub`；且未注册 Discover / Lease 工厂，
   该驱动当前仅能创建实例，无法完成任何协调操作。
4. **`etcd` Discovery 未解码实例值**：源码留有 `TODO: decode ServiceInstance JSON from kv.Value`，
   返回的是占位 `ServiceInstance`；`Watch` 触发回调时复用 `Discovery`，因此回调数据同样受限。
5. **`consul` Watch 为空实现**：仅记录日志并返回 `nil`，不会调用 callback；`consul` 当前只能做一次性 `Discovery`。
6. **`k8s_configmap` 只有 Register 工厂**：无 Discover / Lease，注册语义是「以 service 名创建一个 ConfigMap」。
7. **`ReadAndCreate` 要求三个工厂齐备**，因此在 etcd / consul 上会因 `NewLease` 失败而整体返回错误。
8. **多个配置字段尚未接入**（见 [5.4](#54-默认值与字段实际生效状态)）：`Secret`、`username`/`password`/`token`、`ttl`、
   `cluster[].namespace`、`cluster[].watch_enabled` 目前不会影响驱动行为。

---

## 8. 可观测性

### 8.1 日志（logx）

各驱动构造时通过 `logx.GetGlobal()` 获取全局 logger，操作日志统一使用 ctx 字段：

```go
d.logger.InfoContext(ctx, "coordx/etcd register done",
	logx.String("service", service),
	logx.String("inst_id", inst.ID),
)
d.logger.ErrorContext(ctx, "coordx/etcd register failed",
	logx.Err(err), logx.String("service", service),
)
```

- 日志格式 / 输出目标由 `logx.ReadConfig()` 决定，各驱动已空导入 `_ "github.com/gospacex/logx/zap"`
- 关键事件前缀统一为 `coordx/<driver>`：`register initialized` / `register done` / `discovery done` / `watch started` 等

### 8.2 链路追踪（traceflag + OTel）

span 名与属性（源码中逐一存在）：

| span 名 | 触发位置 |
|---|---|
| `coordx.etcd.register.init` / `coordx.etcd.discover.init` | etcd 构造（目前仅 etcd 有 init span） |
| `coord.register` | `Register()` |
| `coord.deregister` | `Deregister()` |
| `coord.discover` | `Discovery()` |
| `coord.watch` | `Watch()` |

常用 attribute：`coord.driver`、`coord.service`、`coord.instance_id`、`coord.key`（etcd）、
`coord.namespace`（k8s_configmap）、`coord.instance_count`、`coordx.driver`（init span）。

**三级开关（AND 关系，全部打开才产生 span）**：

1. 实例级：`coordx.Config.EnableTrace`
2. 组件级：`traceflag.SetComponent("coordx", true)`（tracer 构造时绑定组件名 `coordx`）
3. 全局总开关：`traceflag.SetEnabled(true)`，**默认关闭**

Provider / exporter 由宿主装配，`examples/etcd/main.go` 中的装配方式：

```go
exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(
	jaeger.WithEndpoint("http://localhost:14268/api/traces")))
tp := sdktrace.NewTracerProvider(
	sdktrace.WithBatcher(exporter),
	sdktrace.WithResource(resource.Default()),
	sdktrace.WithSampler(sdktrace.AlwaysSample()),
)
otel.SetTracerProvider(tp)
```

装配完成后还需显式打开全局总开关（默认关闭），否则组件级 / 实例级开关均不生效：

```go
traceflag.SetEnabled(true)
```

> 指标（Metrics）当前未在 coordx 中暴露。

---

## 9. 示例与文档

### 9.1 示例目录

| 路径 | 内容 |
|---|---|
| `examples/etcd/main.go` | 完整流程：logx 初始化 → `ReadAndCreate` → 健康检查 → 注册 2 个实例 → `Discovery` → `Watch` → `Grant`/`Renew`/`Revoke` → 注销（含 Jaeger tracer 装配） |
| `examples/etcd/config.yaml` | 上文 4.3 引用的真实配置 |
| `examples/etcd/etcd` | 上次构建的可执行产物（Mach-O 二进制） |

> 运行示例需先满足 etcd 连接；示例中的 `lease` 调用受 [7.2](#72-已知限制) 第 1、7 条限制。

### 9.2 本地环境（docker-compose.yml）

`docker-compose.yml` 定义了 5 个服务（镜像统一走 `swr.cn-north-4.myhuaweicloud.com` 前缀），网络名 `hubx-coordx-net`：

| 服务 | 容器名 | 端口 | 说明 |
|---|---|---|---|
| mysql | hubx-coord-mysql | 3306 | Apollo QuickStart 依赖 |
| nacos | hubx-nacos | 8848 / 9848 / 9849 | standalone 模式 |
| consul | hubx-consul | 8500 | server-1 单节点 + UI |
| etcd | hubx-etcd | 2379 / 2380 | v3.5.0，带 healthcheck |
| apollo | hubx-apollo | 8070 / 8085→8080 | apollo-quick-start 2.4.0 |

### 9.3 端到端校验（verify.sh）

```bash
./verify.sh          # 默认 driver=etcd
./verify.sh consul   # 指定 driver
```

脚本流程：`docker compose up -d` → 轮询等待 `hubx-etcd` healthcheck healthy（最多 30 次 × 2s）→
`cd examples/<driver> && go run .` → `docker compose down`。

> 脚本内 `REPO_ROOT` 与 Go 路径写死为本机路径，迁移到其他机器需先修改。

### 9.4 驱动能力矩阵草稿

各驱动目录下另有 `FEATURE_MATRIX.md`（`apollo/` `consul/` `etcd/` `nacos/` `k8s_configmap/`）。
**其中的能力结论为早期草稿，与当前源码不一致处以本 README 第 7 节为准。**

---

## 10. 测试与质量

```bash
# 根模块（含 examples/etcd）
go build ./...
go vet ./...
go test ./...

# 五个驱动子模块各自独立编译
cd apollo         && go build ./...
cd consul         && go build ./...
cd etcd           && go build ./...
cd nacos          && go build ./...
cd k8s_configmap  && go build ./...
```

- 根模块与五个子模块 `go build` / `go vet` 均可直接通过
- 当前仓库内**没有 `*_test.go` 测试文件**，`go test ./...` 输出 `[no test files]`
- 端到端行为验证见 `./verify.sh`（依赖 Docker，见 [9.3](#93-端到端校验verifysh)）

---

## 11. 版本与兼容性

| 项 | 值 |
|---|---|
| Module | `github.com/gospacex/coordx` |
| Go 版本 | `go 1.22`（`k8s_configmap` 子模块声明 `go 1.22.0`） |
| 当前 tag | `v0.0.1` |
| 模块布局 | 根模块 + 5 个独立子模块（`apollo` `consul` `etcd` `nacos` `k8s_configmap` 各带 `go.mod`） |
| 关键依赖 | `go.etcd.io/etcd/client/v3 v3.5.16`、`github.com/hashicorp/consul/api v1.29.2`、`github.com/nacos-group/nacos-sdk-go/v2 v2.2.5`、`k8s.io/client-go v0.30.3`、`go.opentelemetry.io/otel v1.29.0` |
| 配置依赖 | `github.com/gospacex/configx`、`github.com/gospacex/logx`、`github.com/gospacex/traceflag` |

> 根 `go.mod` 中的 `replace` 指令将子模块与同系列 SDK 指向仓库相对路径，用于本地开发；
> 对外发布时由各模块自身的 tag 解析。

---

## 12. 许可证

MIT License
