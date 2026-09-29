// k8s_configmap/register.go - 驱动注册
package k8sconfigmap

import "github.com/gospacex/coordx"

func init() {
	coordx.RegisterCreator("k8s_configmap", func(cfg *coordx.Config) (coordx.Register, error) {
		return NewRegister(cfg)
	})
}
