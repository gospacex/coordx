// coordx/k8s_configmap - K8s ConfigMap 协调驱动配置（Standalone + Cluster）
package k8sconfigmap

import (
	"github.com/gospacex/coordx"
)

// StandaloneConfig K8s ConfigMap 单实例配置
type StandaloneConfig struct {
	Driver      string `yaml:"driver" json:"driver"`
	Name        string `yaml:"name" json:"name"`
	Addr        string `yaml:"addr" json:"addr"`
	Namespace   string `yaml:"namespace" json:"namespace"`
	EnableTrace bool   `yaml:"enable_trace" json:"enable_trace"`
}

// ClusterConfig K8s ConfigMap 集群配置
type ClusterConfig struct {
	Driver     string   `yaml:"driver" json:"driver"`
	Name       string   `yaml:"name" json:"name"`
	Addr       string   `yaml:"addr" json:"addr"`
	Endpoints  []string `yaml:"endpoints" json:"endpoints"`
	Namespace  string   `yaml:"namespace" json:"namespace"`
	Context    string   `yaml:"context" json:"context"`
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
		Driver:      "k8s_configmap",
		Name:        n,
		Addr:        s.Addr,
		Cluster:     s.Namespace,
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
		Driver:      "k8s_configmap",
		Name:        n,
		Addr:        addr,
		Cluster:     c.Namespace,
	}
}
