package trace_module

import (
	"context"
	"fmt"
	"time"

	"contrib.go.opencensus.io/exporter/stackdriver"
	"github.com/lixin9311/micro/cfg_module"
	"github.com/lixin9311/micro/svc_module"
	"github.com/spf13/viper"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
	"go.uber.org/fx"
	"go.uber.org/zap"
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

func (noopExporter) ExportSpan(s *trace.SpanData) {}

func NewTraceExporter(ctx context.Context, lc fx.Lifecycle, cfg Config, logger *zap.Logger, svcCfg svc_module.OptionalConfig) (trace.Exporter, error) {
	driver := cfg.Driver
	switch driver {
	case "", "none":
		return noopExporter{}, nil
	case "stackdriver":
		projectID := svcCfg.GetProjectID()
		if projectID == "unknown" {
			return nil, fmt.Errorf("project id must be set for stackdriver exporter")
		}
		sd, err := stackdriver.NewExporter(stackdriver.Options{
			ProjectID:         projectID,
			OnError:           func(err error) { logger.Error("failed to export to stackdriver", zap.Error(err)) },
			MetricPrefix:      svcCfg.GetDomain(),
			ReportingInterval: 90 * time.Second,
			Context:           ctx,
		})
		if err != nil {
			return nil, fmt.Errorf("unable to create stackdriver exporter: %w", err)
		}
		lc.Append(fx.Hook{
			OnStart: func(context.Context) error {
				logger.Info("Starting stackdriver exporter")
				if err := sd.StartMetricsExporter(); err != nil {
					return fmt.Errorf("failed to start stackdriver metrics exporter: %w", err)
				}
				return nil
			},
			OnStop: func(ctx context.Context) error {
				logger.Info("Shuting down stackdriver exporter")
				sd.StopMetricsExporter()
				sd.Flush()
				return nil
			},
		})
		return sd, err
	default:
		return nil, fmt.Errorf("unknown trace driver: %s", driver)
	}
}

type opencensusViewsParams struct {
	fx.In

	Views []*view.View `group:"opencensus_views"`
}

type openCensusViewsOptions struct {
	fx.Out

	Views []*view.View `group:"opencensus_views,flatten"`
}

func WithOpencensusViews(views ...*view.View) fx.Option {
	return fx.Supply(openCensusViewsOptions{Views: views})
}

func RegisterTrace(e trace.Exporter, opts opencensusViewsParams) error {
	if len(opts.Views) != 0 {
		if err := view.Register(opts.Views...); err != nil {
			return fmt.Errorf("failed to register custom views: %w", err)
		}
	}
	trace.RegisterExporter(e)
	return nil
}
