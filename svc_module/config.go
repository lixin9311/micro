package svc_module

import (
	"go.uber.org/fx"
)

type Service string
type Domain string
type ProjectID string

type OptionalConfig struct {
	fx.In

	ProjectID ProjectID `optional:"true"`
	Service   Service   `optional:"true"`
	Domain    Domain    `optional:"true"`
}

func (cfg OptionalConfig) GetService() string {
	if cfg.Service == "" {
		return "unknwon"
	}
	return string(cfg.Service)
}

func (cfg OptionalConfig) GetProjectID() string {
	if cfg.ProjectID == "" {
		return "unknwon"
	}
	return string(cfg.ProjectID)
}

func (cfg OptionalConfig) GetDomain() string {
	if cfg.Domain == "" {
		return "unknwon"
	}
	return string(cfg.Domain)
}

func Module(service, domain string) fx.Option {
	return fx.Options(
		fx.Supply(
			Service(service),
			Domain(domain),
		),
	)
}

func WithProjectID(projectID string) fx.Option {
	return fx.Supply(ProjectID(projectID))
}
