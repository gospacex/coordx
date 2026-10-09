// consul/register.go - 驱动注册
package consul

import "github.com/gospacex/coordx"

func init() {
	coordx.RegisterCreator("consul", func(cfg *coordx.Config) (coordx.Register, error) {
		return NewRegister(cfg)
	})
	coordx.RegisterDiscoverCreator("consul", func(cfg *coordx.Config) (coordx.Discover, error) {
		return NewDiscover(cfg)
	})
	// Lease 此前未注册：coordx.ReadAndCreate 无条件创建三件套，缺一个就整体
	// 失败，consul 因此走不了统一入口。Consul Session 即租约原语，注册真实实现。
	coordx.RegisterLeaseCreator("consul", func(cfg *coordx.Config) (coordx.Lease, error) {
		return NewLease(cfg)
	})
}
