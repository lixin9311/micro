package http_module

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"github.com/lixin9311/micro/cfg_module"
	"github.com/lixin9311/micro/http_middleware"
	"github.com/lixin9311/micro/svc_module"
	"github.com/lixin9311/micro/trace_module"
	"github.com/lixin9311/micro/utils"
	"github.com/lixin9311/micro/version"
	"github.com/spf13/viper"
	"go.opencensus.io/exporter/stackdriver/propagation"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/trace"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
)

type beforeHttp struct{}

type optionalParams struct {
	fx.In

	H2c      *http2.Server       `optional:"true"`
	TraceCfg trace_module.Config `optional:"true"`
	Before   []beforeHttp        `group:"before_http"`
}

type HttpOptions struct {
	fx.Out

	Before []beforeHttp `group:"before_http,flatten"`
}

func BeforeHttp() HttpOptions {
	return HttpOptions{
		Before: []beforeHttp{{}},
	}
}

type Config struct {
	ListenAddr    string      `mapstructure:"listen-addr" validate:"required,ip"`
	ListenPort    int         `mapstructure:"listen-port" validate:"required,gt=0,lte=65535"`
	LogAllRequest bool        `mapstructure:"log-all-request"`
	CORS          CorsSetting `mapstructure:"cors"`
}

type CorsSetting struct {
	AllowOrigins []string `mapstructure:"allowed-origins"`
	AllowHeaders []string `mapstructure:"allowed-header"`
}

var DefaultConfig = wrappedCfg{
	Http: Config{
		ListenAddr:    "0.0.0.0",
		ListenPort:    3000,
		LogAllRequest: true,
		CORS: CorsSetting{
			AllowOrigins: []string{"*"},
			AllowHeaders: []string{"Accept", "Content-Type", "Content-Length", "Accept-Encoding", "Authorization", "ResponseType"},
		},
	},
}

type wrappedCfg struct {
	Http Config `mapstructure:"http"`
}

func ReadConfig(v *viper.Viper) (Config, error) {
	cfg := &wrappedCfg{}
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, err
	}
	cfg.Http.ListenPort = utils.GetDefaultPort("http", cfg.Http.ListenPort)
	return cfg.Http, nil
}

func CheckConfig(cfg Config) error {
	return validator.New().Struct(&cfg)
}

func Module(force bool) fx.Option {
	if force {
		return fx.Options(
			cfg_module.SetDefaultConfig(DefaultConfig),
			fx.Provide(
				ReadConfig,
				NewEcho,
			),
			fx.Invoke(
				CheckConfig,
				noop,
			),
		)
	}
	return fx.Options(
		cfg_module.SetDefaultConfig(DefaultConfig),
		fx.Provide(
			ReadConfig,
			NewEcho,
		),
		fx.Invoke(
			CheckConfig,
		),
	)
}

// need to invoke echo
func noop(e *echo.Echo) {}

func NewEcho(
	logger *zap.Logger,
	lc fx.Lifecycle,
	cfg Config,
	serviceParams svc_module.OptionalConfig,
	ocfg optionalParams,
) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Logger.SetLevel(log.OFF)
	e.Server.ReadTimeout = time.Minute
	e.Server.WriteTimeout = 2 * time.Minute
	service := string(serviceParams.Service)
	if service == "" {
		service = "unknown"
	}

	e.Use(
		middleware.CORSWithConfig(middleware.CORSConfig{
			AllowCredentials: true,
			AllowOrigins:     cfg.CORS.AllowOrigins,
			AllowHeaders:     cfg.CORS.AllowHeaders,
		}),
		http_middleware.EchoRequestID(),
		http_middleware.EchoRequestLogger(
			logger,
			http_middleware.WithLogBody(cfg.LogAllRequest),
		),
	)

	if ocfg.TraceCfg.Driver != "none" {
		e.Use(
			http_middleware.WrapMiddleware(
				echo.WrapMiddleware(func(h http.Handler) http.Handler {
					return &ochttp.Handler{
						Propagation: &propagation.HTTPFormat{},
						Handler:     h,
						StartOptions: trace.StartOptions{
							Sampler:  trace.ProbabilitySampler(ocfg.TraceCfg.Fraction),
							SpanKind: trace.SpanKindServer,
						},
						IsPublicEndpoint: os.Getenv("K_SERVICE") != "",
					}
				}),
			),
		)
	}

	p := prometheus.NewPrometheus(service, func(c echo.Context) bool {
		switch c.Request().RequestURI {
		case "", "/", "/metrics", "/healthz":
			return true
		default:
			return false
		}
	})
	p.Use(e)

	domain := string(serviceParams.Domain)
	if domain == "" {
		domain = "unknown"
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			addr := fmt.Sprintf("%s:%d", cfg.ListenAddr, cfg.ListenPort)
			registeredRoute := map[string]bool{}
			for _, r := range e.Routes() {
				registeredRoute[r.Method+"|"+r.Path] = true
			}
			if !registeredRoute["GET|/"] {
				logger.Debug(fmt.Sprintf("register %s with default handler", "/"))
				e.GET("/", func(c echo.Context) error {
					return c.String(http.StatusOK, domain+":"+version.Version())
				})
			}
			if !registeredRoute["GET|/healthz"] {
				logger.Debug(fmt.Sprintf("register %s with default handler", "/healthz"))
				e.GET("/healthz", func(c echo.Context) error {
					return c.String(http.StatusOK, domain+":"+version.Version())
				})
			}
			logger.Info("Starting HTTP server on " + addr)
			// In production, we'd want to separate the Listen and Serve phases for
			// better error-handling.
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("failed to serve HTTP service: %w", err)
			}
			e.Listener = ln
			go func() {
				if ocfg.H2c != nil {
					if err := e.StartH2CServer(addr, ocfg.H2c); err != nil && err != http.ErrServerClosed {
						logger.Panic("error during serving HTTP/2", zap.Error(err))
					}
				} else {
					if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
						logger.Panic("error during serving HTTP", zap.Error(err))
					}
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Stopping HTTP server")
			return e.Shutdown(ctx)
		},
	})

	return e
}
