// etcd/register.go - 驱动注册
package etcd

import "github.com/gospacex/coordx"

func init() {
	coordx.RegisterCreator("etcd", func(cfg *coordx.Config) (coordx.Register, error) {
		return NewRegister(cfg)
	})
	coordx.RegisterDiscoverCreator("etcd", func(cfg *coordx.Config) (coordx.Discover, error) {
		return NewDiscover(cfg)
	})
}
