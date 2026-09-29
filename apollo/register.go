// apollo/register.go - 驱动注册
package apollo

import "github.com/gospacex/coordx"

func init() {
	coordx.RegisterCreator("apollo", func(cfg *coordx.Config) (coordx.Register, error) {
		return NewRegister(cfg)
	})
}
