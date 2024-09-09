package general

import (
	"context"
	ConfigBuilder "github.com/keloran/go-config"
)

type System struct {
	Config  *ConfigBuilder.Config
	Context context.Context
}

//func NewSystem(cfg *ConfigBuilder.Config) *System {
//	return &System{
//		Config: cfg,
//	}
//}

func (s *System) SetContext(ctx context.Context) *System {
	s.Context = ctx
	return s
}
