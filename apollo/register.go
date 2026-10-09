// apollo/register.go - 驱动注册
package apollo

import "github.com/gospacex/coordx"

func init() {
	coordx.RegisterCreator("apollo", func(cfg *coordx.Config) (coordx.Register, error) {
		return NewRegister(cfg)
	})
	// Discover / Lease 同样是 Apollo 不具备的能力，但仍要注册：
	// coordx.ReadAndCreate 会无条件创建三件套，缺一个就整体失败，
	// 驱动便无法通过统一入口使用。注册为"显式不可用"实现后，调用方
	// 能用 errors.Is(err, coordx.ErrUnsupported) 精确降级。
	coordx.RegisterDiscoverCreator("apollo", func(cfg *coordx.Config) (coordx.Discover, error) {
		return NewDiscover(cfg)
	})
	coordx.RegisterLeaseCreator("apollo", func(cfg *coordx.Config) (coordx.Lease, error) {
		return NewLease(cfg)
	})
}
