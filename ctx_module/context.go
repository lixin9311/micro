package ctx_module

import (
	"context"

	"go.uber.org/fx"
)

func Module(ctx context.Context) fx.Option {
	return fx.Provide(func(lc fx.Lifecycle) context.Context {
		ctx, cancel := context.WithCancel(ctx)
		lc.Append(fx.Hook{
			OnStop: func(ctx context.Context) error {
				cancel()
				return nil
			},
		})
		return ctx
	})
}
