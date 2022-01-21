package main

import (
	"context"
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/grpc-ecosystem/go-grpc-middleware/logging/zap/ctxzap"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"google.golang.org/grpc"

	"github.com/lixin9311/micro/cfg_module"
	"github.com/lixin9311/micro/cmd_module"
	"github.com/lixin9311/micro/ctx_module"
	example "github.com/lixin9311/micro/example/proto"
	"github.com/lixin9311/micro/gateway_module"
	"github.com/lixin9311/micro/grpc_module"
	"github.com/lixin9311/micro/http_module"
	"github.com/lixin9311/micro/svc_module"
	"github.com/lixin9311/micro/trace_module"
	"github.com/lixin9311/micro/zap_module"
)

// Define a viper compatible config struct
type Config struct {
	WelcomeMessage string `mapstructure:"welcome-message" validate:"required"`
}

var DefaultConfig = Config{
	WelcomeMessage: "hello",
}

// should read config from a viper instance
func readConfig(v *viper.Viper) (Config, error) {
	cfg := Config{}
	if err := v.Unmarshal(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func checkConfig(cfg Config) error {
	return validator.New().Struct(&cfg)
}

type server struct {
	example.UnimplementedGreeterServer
	msg string
}

func (s *server) Hello(ctx context.Context, req *example.HelloReq) (resp *example.HelloResp, err error) {
	ctxzap.Info(ctx, "received request")
	return &example.HelloResp{
		Message: s.msg + " " + req.GetMessage(),
	}, nil
}

func NewGRPCService(cfg Config) example.GreeterServer {
	return &server{msg: cfg.WelcomeMessage}
}

func RegisterGRPCService(srv *grpc.Server, svc example.GreeterServer) {
	example.RegisterGreeterServer(srv, svc)
}

func RegisterGRPCGateway(lc fx.Lifecycle, cfg grpc_module.Config, gwmux *runtime.ServeMux) error {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) (err error) {
			return example.RegisterGreeterHandler(context.Background(),
				gwmux,
				grpc_module.MustDial(fmt.Sprintf("127.0.0.1:%d", cfg.ListenPort), grpc.WithInsecure()),
			)
		},
	})
	return nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := fx.New(
		svc_module.Module("example", "example.example.com"),
		svc_module.WithProjectID("project-example"),
		cmd_module.Module(false),
		cfg_module.Module(""),
		ctx_module.Module(ctx),
		zap_module.Module(),
		trace_module.Module(),
		grpc_module.Module(),
		http_module.Module(true),
		gateway_module.Module(),

		cfg_module.SetDefaultConfig(DefaultConfig),
		fx.Provide(
			readConfig,
			NewGRPCService,
		),
		fx.Invoke(
			RegisterGRPCService,
			RegisterGRPCGateway,
			checkConfig,
		),
	)
	app.Run()
}
