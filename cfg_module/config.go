package cfg_module

import (
	"github.com/lixin9311/micro/viperutil"
	"github.com/spf13/viper"
	"go.uber.org/fx"
)

var Viper = viper.New()

type defaultCfgOptionsParams struct {
	fx.In

	Options []interface{} `group:"default_configs"`
}

type defaultCfgOptions struct {
	fx.Out

	Options interface{} `group:"default_configs"`
}

func SetDefaultConfig(in interface{}) fx.Option {
	return fx.Supply(defaultCfgOptions{Options: in})
}

func Module(path string) fx.Option {
	return fx.Options(
		fx.Provide(
			ReadConfig(path),
		),
	)
}

func ReadConfig(path string) func(opts defaultCfgOptionsParams) (*viper.Viper, error) {
	return func(opts defaultCfgOptionsParams) (*viper.Viper, error) {
		for _, opt := range opts.Options {
			viperutil.VSetDefault(Viper, opt)
		}
		// only load config once (whether via direct call or fx.Module)
		if path != "" {
			Viper.SetConfigFile(path)
			if err := Viper.ReadInConfig(); err != nil {
				return nil, err
			}
		}
		return Viper, nil
	}
}
