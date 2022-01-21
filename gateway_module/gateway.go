package gateway_module

import (
	"github.com/go-playground/validator/v10"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/labstack/echo/v4"
	"github.com/lixin9311/micro/cfg_module"
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"google.golang.org/protobuf/encoding/protojson"
)

type Config struct {
	UseProtoNames   bool `mapstructure:"use-proto-names"`
	EmitUnpopulated bool `mapstructure:"emit-unpopulated"`
	DiscardUnknown  bool `mapstructure:"discard-unknown"`
}

var DefaultConfig = wrappedCfg{
	GW: Config{
		UseProtoNames:   true,
		EmitUnpopulated: true,
		DiscardUnknown:  true,
	},
}

type wrappedCfg struct {
	GW Config `mapstructure:"gateway"`
}

func ReadConfig(v *viper.Viper) (Config, error) {
	cfg := &wrappedCfg{}
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, err
	}
	return cfg.GW, nil
}

func CheckConfig(cfg Config) error {
	return validator.New().Struct(&cfg)
}

func Module() fx.Option {
	return fx.Options(
		cfg_module.SetDefaultConfig(DefaultConfig),
		fx.Provide(
			ReadConfig,
			NewGatewayMux,
		),
		fx.Invoke(
			RegisterGateway,
		),
	)
}

func customMatcher(key string) (string, bool) {
	switch key {
	case "x-request-id":
		return key, true
	default:
		return runtime.DefaultHeaderMatcher(key)
	}
}

type runtimeOptionsParams struct {
	fx.In

	Options []runtime.ServeMuxOption `group:"grpc_gateway_options"`
}

type runtimeOptions struct {
	fx.Out

	Options []runtime.ServeMuxOption `group:"grpc_gateway_options,flatten"`
}

func WithGWOptions(opts ...runtime.ServeMuxOption) fx.Option {
	return fx.Supply(runtimeOptions{Options: opts})
}

func NewGatewayMux(cfg Config, opts runtimeOptionsParams) *runtime.ServeMux {
	// f := func(err error) (codes.Code, proto.Message) {
	// 	pb := errorpb.MustFromError(err)
	// 	return errorpb.Code(pb), pb
	// }
	// statusToErr := func(httpStatus int) error {
	// 	err := errorpb.New(codes.Internal, errorpb.ID_INTERNAL).WithReason("Unexpected routing error")
	// 	switch httpStatus {
	// 	case http.StatusBadRequest:
	// 		err.WithCode(codes.InvalidArgument).WithReason(http.StatusText(httpStatus))
	// 	case http.StatusMethodNotAllowed:
	// 		err.WithCode(codes.Unimplemented).WithReason(http.StatusText(httpStatus))
	// 	case http.StatusNotFound:
	// 		err.WithCode(codes.NotFound).WithReason(http.StatusText(httpStatus))
	// 	}
	// 	return err
	// }

	gwopts := []runtime.ServeMuxOption{
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.HTTPBodyMarshaler{
			Marshaler: &runtime.JSONPb{
				MarshalOptions: protojson.MarshalOptions{
					UseProtoNames:   cfg.UseProtoNames,
					EmitUnpopulated: cfg.EmitUnpopulated,
				},
				UnmarshalOptions: protojson.UnmarshalOptions{
					DiscardUnknown: cfg.DiscardUnknown,
				},
			},
		}),
		runtime.WithIncomingHeaderMatcher(customMatcher),
	}
	gwopts = append(gwopts, opts.Options...)

	gwmux := runtime.NewServeMux(
		gwopts...,
	// runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.HTTPBodyMarshaler{
	// 	Marshaler: &runtime.JSONPb{
	// 		MarshalOptions: protojson.MarshalOptions{
	// 			UseProtoNames:   cfg.UseProtoNames,
	// 			EmitUnpopulated: cfg.EmitUnpopulated,
	// 		},
	// 		UnmarshalOptions: protojson.UnmarshalOptions{
	// 			DiscardUnknown: cfg.DiscardUnknown,
	// 		},
	// 	},
	// }),
	// runtime.WithErrorHandler(gateway_middleware.NewHTTPErrorHandler(f)),
	// runtime.WithRoutingErrorHandler(gateway_middleware.NewRoutingErrorHandler(statusToErr, f)),
	// runtime.WithIncomingHeaderMatcher(customMatcher),
	)

	return gwmux
}

func RegisterGateway(e *echo.Echo, gwmux *runtime.ServeMux) {
	e.Any("/*", echo.WrapHandler(gwmux))
}
