package zap_module

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	"github.com/lixin9311/micro/cfg_module"
	"github.com/lixin9311/micro/svc_module"
	"github.com/lixin9311/micro/version"
	"github.com/lixin9311/zapx"
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var DefaultConfig = wrappedCfg{
	Log: Config{
		Driver: "development",
		Level:  "debug",
	},
}

type Config struct {
	Driver       string `mapstructure:"driver"` // development, stackdriver
	Level        string `mapstructure:"level"`  // debug, info, warn, error, panic, fatal
	SlackWebhook string `mapstructure:"slack-webhook" validate:"omitempty,url"`
}

type wrappedCfg struct {
	Log Config `mapstructure:"log"`
}

func ReadConfig(v *viper.Viper) (Config, error) {
	cfg := &wrappedCfg{}
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, err
	}
	return cfg.Log, nil
}

func CheckConfig(cfg Config) error {
	return validator.New().Struct(&cfg)
}

func Module() fx.Option {
	return fx.Options(
		cfg_module.SetDefaultConfig(DefaultConfig),
		fx.Provide(
			ReadConfig,
			newLogger,
		),
		fx.Invoke(
			CheckConfig,
			ReplaceGlobalLogger,
		),
	)
}

func getLogLevel(level string) (zapcore.Level, error) {
	switch level {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	case "panic":
		return zapcore.PanicLevel, nil
	case "fatal":
		return zap.PanicLevel, nil
	default:
		return 0, fmt.Errorf("unable to parse log level: %s", level)
	}
}

func NewLogger(lvl zapcore.Level, driver string, opts ...zapx.Option) (logger *zap.Logger, err error) {
	if driver == "stackdriver" {
		logger = zapx.Zap(lvl,
			opts...,
		)
		grpclogger := zapx.Zap(zapcore.WarnLevel,
			opts...,
		)
		grpc_zap.ReplaceGrpcLoggerV2(grpclogger.Named("grpclog").WithOptions(
			zap.AddCallerSkip(4),
		))
	} else if driver == "development" {
		logger, err = zap.NewDevelopment(zap.IncreaseLevel(lvl))
		grpc_zap.ReplaceGrpcLoggerV2(zap.NewNop())
		// grpc_zap.ReplaceGrpcLoggerV2(logger.Named("grpclog").WithOptions(
		// 	zap.AddCallerSkip(4),
		// ))
	} else {
		return nil, fmt.Errorf("unknown log driver: %s", driver)
	}

	return logger, err
}

type zapxOptionsParams struct {
	fx.In

	Options []zapx.Option `group:"zapx_options"`
}

type zapxOptions struct {
	fx.Out

	Options []zapx.Option `group:"zapx_options,flatten"`
}

func WithOptions(opts ...zapx.Option) fx.Option {
	return fx.Supply(zapxOptions{
		Options: opts,
	})
}

func newLogger(lc fx.Lifecycle, cfg Config, serviceParams svc_module.OptionalConfig, opts zapxOptionsParams) (logger *zap.Logger, err error) {
	driver := cfg.Driver
	lvl, err := getLogLevel(cfg.Level)
	if err != nil {
		return nil, err
	}
	ver := version.Version()
	zopts := []zapx.Option{
		zapx.WithService(serviceParams.GetService()),
		zapx.WithSlackURL(cfg.SlackWebhook),
		zapx.WithVersion(ver),
	}
	zopts = append(zopts, opts.Options...)
	logger, err = NewLogger(lvl, driver, zopts...)
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Debug("Syncing logger")
			err := logger.Sync()
			// https://github.com/uber-go/zap/issues/772
			if strings.Contains(err.Error(), "/dev/stdout") || strings.Contains(err.Error(), "/dev/stderr") {
				return nil
			}
			return err
		},
	})
	return logger, err
}

func ReplaceGlobalLogger(l *zap.Logger) {
	zap.ReplaceGlobals(l)
}
