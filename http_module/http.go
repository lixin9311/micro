package http_module

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"github.com/lixin9311/zapx"
	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"pkg.lucas.icu/micro/cfg_module"
	"pkg.lucas.icu/micro/http_middleware"
	"pkg.lucas.icu/micro/svc_module"
	"pkg.lucas.icu/micro/trace_module"
	"pkg.lucas.icu/micro/utils"
	"pkg.lucas.icu/micro/version"
)

type beforeHttp struct{}

type optionalParams struct {
	fx.In

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
	ListenAddr     string      `mapstructure:"listen-addr" validate:"required,ip"`
	ListenPort     int         `mapstructure:"listen-port" validate:"required,gt=0,lte=65535"`
	LogAllRequest  bool        `mapstructure:"log-all-request"`
	LogIgnorePaths []string    `mapstructure:"log-ignore-paths"`
	CORS           CorsSetting `mapstructure:"cors"`
	H2c            bool        `mapstructure:"h2c"`
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
		H2c: false,
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
		middleware.RecoverWithConfig(middleware.RecoverConfig{
			LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
				logger.Error("recover from panic",
					zapx.Request(zapx.HTTPRequestEntry{
						Request: c.Request(),
						Status:  500,
					}),
					zap.String("path", c.Path()),
					zap.Error(err),
					zap.String("stack", string(stack)),
				)
				return nil
			},
		}),
		http_middleware.EchoRequestID(),
		middleware.CORSWithConfig(middleware.CORSConfig{
			AllowCredentials: true,
			AllowOrigins:     cfg.CORS.AllowOrigins,
			AllowHeaders:     cfg.CORS.AllowHeaders,
		}),
	)

	if ocfg.TraceCfg.Fraction > 0 && ocfg.TraceCfg.Driver != "none" {
		// TODO: here
		skippedUrl := map[string]bool{}
		for _, p := range cfg.LogIgnorePaths {
			skippedUrl[p] = true
		}
		skipper := otelecho.WithSkipper(
			func(c echo.Context) bool {
				return skippedUrl[c.Path()]
			})
		e.Use(otelecho.Middleware(service, skipper))
	}

	if cfg.H2c {
		e.Use(
			http_middleware.EchoRequestLogger(
				logger,
				http_middleware.WithLogBody(false), // do not use log body for h2c
				http_middleware.SkipURL(cfg.LogIgnorePaths...),
			),
		)
	} else {
		e.Use(
			http_middleware.EchoRequestLogger(
				logger,
				http_middleware.WithLogBody(cfg.LogAllRequest),
				http_middleware.SkipURL(cfg.LogIgnorePaths...),
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
				if cfg.H2c {
					if err := e.StartH2CServer(addr, &http2.Server{}); err != nil && err != http.ErrServerClosed {
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
