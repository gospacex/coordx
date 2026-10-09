// k8s_configmap/register.go - 驱动注册
package k8sconfigmap

import "github.com/gospacex/coordx"

func init() {
	coordx.RegisterCreator("k8s_configmap", func(cfg *coordx.Config) (coordx.Register, error) {
		return NewRegister(cfg)
	})
	// Discover / Lease 尚未实现，但必须注册：coordx.ReadAndCreate 无条件创建
	// 三件套，缺一个就整体失败。注册为"显式不可用"实现后调用方可用
	// errors.Is(err, coordx.ErrUnsupported) 判定并降级。
	coordx.RegisterDiscoverCreator("k8s_configmap", func(cfg *coordx.Config) (coordx.Discover, error) {
		return NewDiscover(cfg)
	})
	coordx.RegisterLeaseCreator("k8s_configmap", func(cfg *coordx.Config) (coordx.Lease, error) {
		return NewLease(cfg)
	})
}
