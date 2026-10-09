// coordx: 服务发现与协调统一接口，支持 nacos / consul / etcd / apollo / k8s_configmap
package coordx

import (
	"context"
	"fmt"
	"sync"
	"time"

	_ "github.com/gospacex/logx/zap"
)

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

// Lease 分布式协调锁
type Lease interface {
	Grant(ctx context.Context, ttl time.Duration) (string, error)
	Renew(ctx context.Context, sessionID string) error
	Revoke(ctx context.Context, sessionID string) error
	HealthCheck() error
	Close() error
}

// Config 协调配置
type Config struct {
	Driver     string
	Name       string
	Addr       string
	Cluster    string
	Secret     string
	TLSEnabled bool
	// EnableTrace 是否启用 OTel 调用链埋点（实例级开关，与 traceflag 组件级/全局开关为 AND 关系）
	EnableTrace bool
}

// FillDefaults 填充默认值
func (c *Config) FillDefaults() {
	if c.Name == "" {
		c.Name = "default"
	}
}

// ===== Factory =====

// factoriesMu 保护下面三个 creator map。
//
// 背景：REVIEW-REPORT.md 第二优先级第 6 条点名 coordx 需要给 factory map 加
// 锁，但第八节已修复清单只覆盖了 mqx/sqlx/kvx/searchx，coordx 被漏掉。
// 未加锁时并发注册 + 并发创建会触发 Go 运行时的
// "fatal error: concurrent map read and map write" —— 这是无法 recover 的
// 崩溃级故障，比普通数据竞态严重得多（已由 interface_test.go 的
// TestFactoryConcurrentAccess 在 -race 下复现）。
var factoriesMu sync.RWMutex

var (
	registerCreators = map[string]func(*Config) (Register, error){}
	discoverCreators = map[string]func(*Config) (Discover, error){}
	leaseCreators    = map[string]func(*Config) (Lease, error){}
)

// RegisterCreator 注册注册中心创建函数
func RegisterCreator(name string, creator func(*Config) (Register, error)) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	registerCreators[name] = creator
}

// NewRegister 创建注册中心
func NewRegister(cfg *Config) (Register, error) {
	cfg.FillDefaults()
	// 只锁 map 查找；creator 本身放到锁外调用 ——
	// 若 creator 内部又去注册别的驱动（插件化场景），持锁调用会自死锁。
	factoriesMu.RLock()
	creator, ok := registerCreators[cfg.Driver]
	factoriesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("coordx: register driver %q not registered", cfg.Driver)
	}
	return creator(cfg)
}

// RegisterDiscoverCreator 注册发现器创建函数
func RegisterDiscoverCreator(name string, creator func(*Config) (Discover, error)) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	discoverCreators[name] = creator
}

// NewDiscover 创建设发现器
func NewDiscover(cfg *Config) (Discover, error) {
	cfg.FillDefaults()
	factoriesMu.RLock()
	creator, ok := discoverCreators[cfg.Driver]
	factoriesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("coordx: discover driver %q not registered", cfg.Driver)
	}
	return creator(cfg)
}

// RegisterLeaseCreator 注册 Lease 创建函数
func RegisterLeaseCreator(name string, creator func(*Config) (Lease, error)) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	leaseCreators[name] = creator
}

// NewLease 创建 Lease
func NewLease(cfg *Config) (Lease, error) {
	cfg.FillDefaults()
	factoriesMu.RLock()
	creator, ok := leaseCreators[cfg.Driver]
	factoriesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("coordx: lease driver %q not registered", cfg.Driver)
	}
	return creator(cfg)
}

// ReadConfig 加载统一配置句柄（所有 x 系列 SDK 共享同一配置源）
// 详见 coordx/config.go 中的 ReadConfig 与 ReadAndCreate
