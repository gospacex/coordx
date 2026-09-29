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
}
