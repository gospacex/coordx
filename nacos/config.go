// coordx/nacos - Nacos 协调驱动配置（Standalone + Cluster）
package nacos

import (
	"github.com/gospacex/coordx"
)

// StandaloneConfig Nacos 单实例配置
type StandaloneConfig struct {
	Driver      string `yaml:"driver" json:"driver"`
	Name        string `yaml:"name" json:"name"`
	Addr        string `yaml:"addr" json:"addr"`
	Username    string `yaml:"username" json:"username"`
	Password    string `yaml:"password" json:"password"`
	Namespace   string `yaml:"namespace" json:"namespace"`
	TLSEnabled  bool   `yaml:"tls_enabled" json:"tls_enabled"`
	EnableTrace bool   `yaml:"enable_trace" json:"enable_trace"`
}

// ClusterConfig Nacos 集群配置
type ClusterConfig struct {
	Driver     string   `yaml:"driver" json:"driver"`
	Name       string   `yaml:"name" json:"name"`
	Endpoints  []string `yaml:"endpoints" json:"endpoints"`
	Username   string   `yaml:"username" json:"username"`
	Password   string   `yaml:"password" json:"password"`
	Namespace  string   `yaml:"namespace" json:"namespace"`
	TLSEnabled bool     `yaml:"tls_enabled" json:"tls_enabled"`
	// Cluster
	EnableTrace bool `yaml:"enable_trace" json:"enable_trace"`
}

// ToStandaloneConfig 转换为 coordx.Config
func (s *StandaloneConfig) ToStandaloneConfig(name string) *coordx.Config {
	n := name
	if s.Name != "" {
		n = s.Name
	}
	return &coordx.Config{
		EnableTrace: s.EnableTrace,
		Driver:      "nacos",
		Name:        n,
		Addr:        s.Addr,
		Cluster:     s.Namespace,
		TLSEnabled:  s.TLSEnabled,
	}
}

// ToClusterConfig 转换为 coordx.Config
func (c *ClusterConfig) ToClusterConfig(name string) *coordx.Config {
	n := name
	if c.Name != "" {
		n = c.Name
	}
	addr := ""
	if len(c.Endpoints) > 0 {
		addr = c.Endpoints[0]
	}
	return &coordx.Config{
		EnableTrace: c.EnableTrace,
		Driver:      "nacos",
		Name:        n,
		Addr:        addr,
		Cluster:     c.Namespace,
		TLSEnabled:  c.TLSEnabled,
	}
}
