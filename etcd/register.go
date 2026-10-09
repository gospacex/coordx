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
	// Lease 此前未注册：coordx.ReadAndCreate 无条件创建三件套，缺一个就整体
	// 失败，etcd 因此走不了统一入口。etcd 有原生 Lease，这里注册真实实现。
	coordx.RegisterLeaseCreator("etcd", func(cfg *coordx.Config) (coordx.Lease, error) {
		return NewLease(cfg)
	})
}
