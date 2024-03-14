package trace_module

import (
	"context"
	"fmt"

	cloudtrace "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	gcppropagator "github.com/GoogleCloudPlatform/opentelemetry-operations-go/propagator"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/api/option"
	"pkg.lucas.icu/micro/cfg_module"
	"pkg.lucas.icu/micro/svc_module"
)

// Module requires ctx_module, log_module, svc_module if configured with stackdriver exporter
// and cfg_module to provide config
func Module() fx.Option {
	return fx.Options(
		cfg_module.SetDefaultConfig(DefaultConfig),
		fx.Provide(
			ReadConfig,
			NewTraceExporter,
		),
		fx.Invoke(
			RegisterTrace,
		),
	)
}

var DefaultConfig = wrappedCfg{
	Trace: Config{
		Fraction: 1.0,
		Driver:   "none",
	},
}

type Config struct {
	Fraction float64 `mapstructure:"fraction"`
	Driver   string  `mapstructure:"driver"`
}

type wrappedCfg struct {
	Trace Config `mapstructure:"trace"`
}

func ReadConfig(v *viper.Viper) (Config, error) {
	cfg := &wrappedCfg{}
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, err
	}
	return cfg.Trace, nil
}

type noopExporter struct{}

func (noopExporter) ExportSpans(context.Context, []sdktrace.ReadOnlySpan) error { return nil }
func (noopExporter) Shutdown(context.Context) error                             { return nil }

func NewTraceExporter(ctx context.Context, cfg Config, logger *zap.Logger, svcCfg svc_module.OptionalConfig) (sdktrace.SpanExporter, error) {
	driver := cfg.Driver
	switch driver {
	case "", "none":
		return noopExporter{}, nil
	// Start cloud trace(previously called stackdriver) exporter.
	case "stackdriver":
		projectID := svcCfg.GetProjectID()
		if projectID == "unknown" {
			return nil, fmt.Errorf("project id must be set for stackdriver exporter")
		}

		texporter, err := cloudtrace.New(
			cloudtrace.WithProjectID(projectID),
			cloudtrace.WithContext(ctx),
			cloudtrace.WithErrorHandler(otel.ErrorHandler(otel.ErrorHandlerFunc(
				func(err error) { logger.Warn("failed to export to stackdriver", zap.Error(err)) },
			))),
			cloudtrace.WithTraceClientOptions([]option.ClientOption{option.WithTelemetryDisabled()}),
		)

		if err != nil {
			if err != nil {
				return nil, fmt.Errorf("unable to create stackdriver trace exporter: %w", err)
			}
		}

		return texporter, err

	default:
		return nil, fmt.Errorf("unknown trace driver: %s", driver)
	}
}

func NewTraceProvider(ctx context.Context, lc fx.Lifecycle, texporter sdktrace.SpanExporter, cfg Config, svcCfg svc_module.OptionalConfig, logger *zap.Logger) (trace.TracerProvider, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(svcCfg.GetDomain()),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create trace provider: %v", err)
	}

	tracerProvoder := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.Fraction))),
		sdktrace.WithBatcher(texporter),
		sdktrace.WithResource(res))

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("Starting trace provider")
			otel.SetTextMapPropagator(
				propagation.NewCompositeTextMapPropagator(
					// Putting the CloudTraceOneWayPropagator first means the TraceContext propagator
					// takes precedence if both the traceparent and the XCTC headers exist.
					gcppropagator.CloudTraceOneWayPropagator{},
					propagation.TraceContext{},
					propagation.Baggage{},
				))
			otel.SetTracerProvider(tracerProvoder)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("Shuting down trace provider")
			err := tracerProvoder.ForceFlush(ctx)
			if err != nil {
				return fmt.Errorf("error flushing trace provider: %+v", err)
			}
			err = tracerProvoder.Shutdown(ctx)
			if err != nil {
				return fmt.Errorf("error shutting down trace provider: %+v", err)
			}
			return nil
		},
	})
	return tracerProvoder, nil
}

func RegisterTrace(tracerProvoder trace.TracerProvider) error {
	return nil
}
