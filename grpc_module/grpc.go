package grpc_module

import (
	"context"
	"fmt"
	"net"

	"github.com/go-playground/validator/v10"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/lixin9311/micro/cfg_module"
	grpc_validator "github.com/lixin9311/micro/grpc_middleware/grpc_validator"
	grpc_zap "github.com/lixin9311/micro/grpc_middleware/grpc_zap"
	request_id "github.com/lixin9311/micro/grpc_middleware/requestid"
	"github.com/lixin9311/micro/http_module"
	"github.com/lixin9311/micro/svc_module"
	"github.com/lixin9311/micro/trace_module"
	"github.com/lixin9311/micro/utils"
	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type Config struct {
	ListenAddr       string   `mapstructure:"listen-addr" validate:"required,ip"`
	ListenPort       int      `mapstructure:"listen-port" validate:"required,gt=0,lte=65535"`
	LogAllRequest    bool     `mapstructure:"log-all-request"`
	LogIgnoreMethods []string `mapstructure:"log-ignore-methods"`
}

var DefaultConfig = wrappedCfg{
	Grpc: Config{
		ListenAddr:    "0.0.0.0",
		ListenPort:    4000,
		LogAllRequest: true,
	},
}

type wrappedCfg struct {
	Grpc Config `mapstructure:"grpc"`
}

func ReadConfig(v *viper.Viper) (Config, error) {
	cfg := &wrappedCfg{}
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, err
	}
	cfg.Grpc.ListenPort = utils.GetDefaultPort("grpc", cfg.Grpc.ListenPort)
	return cfg.Grpc, nil
}

func CheckConfig(cfg Config) error {
	return validator.New().Struct(&cfg)
}

func Module() fx.Option {
	return fx.Options(
		cfg_module.SetDefaultConfig(DefaultConfig),
		fx.Provide(
			ReadConfig,
			NewGRPCServer,
		),
		// trace_module.WithOpencensusViews(
		// 	ocgrpc.DefaultServerViews...,
		// ),
		// trace_module.WithOpencensusViews(
		// 	ocgrpc.DefaultClientViews...,
		// ),
		// trace_module.WithOpencensusViews(
		// 	ocgrpc.ServerReceivedMessagesPerRPCView,
		// 	ocgrpc.ServerSentMessagesPerRPCView,
		// 	ocgrpc.ClientSentMessagesPerRPCView,
		// 	ocgrpc.ClientReceivedMessagesPerRPCView,
		// 	ocgrpc.ClientServerLatencyView,
		// ),
		fx.Invoke(
			CheckConfig,
		),
	)
}

type GRPCServerOptions struct {
	fx.Out

	Options []grpc.ServerOption `group:"grpc_server_option,flatten"`
}

func WithServerOptions(options ...grpc.ServerOption) fx.Option {
	return fx.Supply(
		GRPCServerOptions{Options: options},
	)
}

type grpcServerOptionsParams struct {
	fx.In

	Options []grpc.ServerOption `group:"grpc_server_option"`
}

type optionalParams struct {
	fx.In

	ValidatorOptions []grpc_validator.Option `optional:"true"`
	RecoveryOptions  []grpc_recovery.Option  `optional:"true"`
	TraceCfg         trace_module.Config     `optional:"true"`
}

func NewGRPCServer(lc fx.Lifecycle, cfg Config, svcCfg svc_module.OptionalConfig, svOpts grpcServerOptionsParams, logger *zap.Logger, ocfg optionalParams) (*grpc.Server, http_module.HttpOptions) {
	ignoredMethods := map[string]bool{}
	for _, m := range cfg.LogIgnoreMethods {
		ignoredMethods[m] = true
	}
	ints := []grpc.UnaryServerInterceptor{
		// insert request id
		request_id.UnaryServerInterceptor(),
		grpc_recovery.UnaryServerInterceptor(ocfg.RecoveryOptions...),
		grpc_zap.UnaryServerInterceptor(logger, cfg.LogAllRequest, func(_ context.Context, m string) bool { return ignoredMethods[m] }),
		grpc_validator.UnaryServerInterceptor(ocfg.ValidatorOptions...),
		grpc_prometheus.UnaryServerInterceptor,
	}

	options := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(ints...),
	}

	if ocfg.TraceCfg.Fraction > 0 && ocfg.TraceCfg.Driver != "none" && ocfg.TraceCfg.Driver != "" {
		options = append(options, grpc.StatsHandler(otelgrpc.NewServerHandler()))
	}
	options = append(options, svOpts.Options...)
	srv := grpc.NewServer(
		options...,
	)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) (err error) {
			if len(srv.GetServiceInfo()) == 0 { // disable server if no service is registered
				return nil
			}
			reflection.Register(srv)
			grpc_prometheus.Register(srv)

			var listen net.Listener
			addr := fmt.Sprintf("%s:%d", cfg.ListenAddr, cfg.ListenPort)
			logger.Info("Starting GRPC server on " + addr)
			listen, err = net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("failed to serve GRPC service: %w", err)
			}
			go func() {
				if err := srv.Serve(listen); err != nil {
					logger.Panic("error during serving GRPC", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if len(srv.GetServiceInfo()) == 0 {
				return nil
			}
			logger.Info("Stopping GRPC server")
			srv.GracefulStop()
			return nil
		},
	})

	// init grpc before http module
	return srv, http_module.BeforeHttp()
}

func MustDial(addr string, opts ...grpc.DialOption) *grpc.ClientConn {
	c, err := Dial(
		addr,
		opts...,
	)
	if err != nil {
		panic(err)
	}
	return c
}

func Dial(addr string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	newOpts := make([]grpc.DialOption, 0, len(opts)+1)
	newOpts = append(newOpts,
		grpc.WithChainUnaryInterceptor(
			request_id.UnaryClientInterceptor(),
			grpc_prometheus.UnaryClientInterceptor,
		),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	newOpts = append(newOpts, opts...)
	return grpc.Dial(
		addr,
		newOpts...,
	)
}
