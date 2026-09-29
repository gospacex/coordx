// coordx/config.go — 统一配置集成（configx + ReadAndCreate）
package coordx

import (
	"fmt"
	"os"

	"github.com/gospacex/configx"
)

// StandaloneConfig 单实例协调配置（本地/viper，适用于单机部署）
type StandaloneConfig struct {
	Driver string `yaml:"driver" json:"driver" mapstructure:"driver"`
	Name   string `yaml:"name" json:"name" mapstructure:"name"`
	Addr   string `yaml:"addr" json:"addr" mapstructure:"addr"`
	// EnableTrace 统一调用链开关（configx 配置键 enable_trace）
	EnableTrace bool `yaml:"enable_trace" json:"enable_trace" mapstructure:"enable_trace"`
}

// ClusterConfig 集群协调配置（Nacos/Apollo 远程配置中心，适用于多节点部署）
type ClusterConfig struct {
	Driver string `yaml:"driver" json:"driver" mapstructure:"driver"`
	Name   string `yaml:"name" json:"name" mapstructure:"name"`
	Addr   string `yaml:"addr" json:"addr" mapstructure:"addr"`
	// Cluster 集群名称
	Cluster string `yaml:"cluster" json:"cluster" mapstructure:"cluster"`
	// Secret 集群密钥
	Secret string `yaml:"secret" json:"secret" mapstructure:"secret"`
	// Namespace 命名空间（多租户隔离）
	Namespace string `yaml:"namespace" json:"namespace" mapstructure:"namespace"`
	// WatchEnabled 是否监听配置变更
	WatchEnabled bool `yaml:"watch_enabled" json:"watch_enabled" mapstructure:"watch_enabled"`
	// EnableTrace 统一调用链开关（configx 配置键 enable_trace）
	EnableTrace bool `yaml:"enable_trace" json:"enable_trace" mapstructure:"enable_trace"`
}

// CoordSection 对应 configx.yaml 中的 coordx 配置节
type CoordSection struct {
	Standalone []*StandaloneConfig `yaml:"standalone"`
	Cluster    []*ClusterConfig    `yaml:"cluster"`
}

// ReadConfig 加载统一配置句柄（所有 x 系列 SDK 共享同一配置源）
func ReadConfig(opts ...configx.ConfigOptions) error {
	_, err := configx.ReadConfig(opts...)
	if err != nil {
		return err
	}
	return nil
}

// ReadAndCreate 从 configx 读取 coordx 配置并创建注册/发现/租约实例
func ReadAndCreate(opts ...configx.ConfigOptions) (register Register, discover Discover, lease Lease, err error) {
	c, err := configx.ReadConfig(opts...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("coordx: failed to read config: %w", err)
	}

	section := &CoordSection{}
	if err := c.Unmarshal("coordx", section); err != nil {
		return nil, nil, nil, fmt.Errorf("coordx: failed to unmarshal coordx section: %w", err)
	}

	var cfg *Config
	for _, cc := range section.Cluster {
		cfg = &Config{
			Driver:      cc.Driver,
			Name:        cc.Name,
			Addr:        cc.Addr,
			Cluster:     cc.Cluster,
			Secret:      cc.Secret,
			EnableTrace: cc.EnableTrace,
		}
		break
	}
	if cfg == nil {
		for _, sc := range section.Standalone {
			cfg = &Config{
				Driver:      sc.Driver,
				Name:        sc.Name,
				Addr:        sc.Addr,
				EnableTrace: sc.EnableTrace,
			}
			break
		}
	}

	if cfg == nil {
		return nil, nil, nil, fmt.Errorf("coordx: no coordx instance configured")
	}

	register, err = NewRegister(cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	discover, err = NewDiscover(cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	lease, err = NewLease(cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	return register, discover, lease, nil
}

// MustReadAndCreate 类似 ReadAndCreate，失败时 panic
func MustReadAndCreate(opts ...configx.ConfigOptions) (register Register, discover Discover, lease Lease) {
	r, d, l, err := ReadAndCreate(opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "coordx: failed to initialize coordinator: %v\n", err)
		os.Exit(1)
	}
	return r, d, l
}
