// nacos/register.go - 驱动注册
package nacos

import "github.com/gospacex/coordx"

func init() {
	coordx.RegisterCreator("nacos", func(cfg *coordx.Config) (coordx.Register, error) {
		return NewRegister(cfg)
	})
	coordx.RegisterDiscoverCreator("nacos", func(cfg *coordx.Config) (coordx.Discover, error) {
		return NewDiscover(cfg)
	})
	coordx.RegisterLeaseCreator("nacos", func(cfg *coordx.Config) (coordx.Lease, error) {
		return NewLease(cfg)
	})
}
