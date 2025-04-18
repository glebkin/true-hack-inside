package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	ServerAddress   string `env:"SERVER_ADDRESS" required:"true"`
	DebugControlUrl string `env:"DEBUG_CONTROL_URL"`

	PprofAddress      string `env:"PPROF_ADDRESS" required:"true"`
	PrometheusAddress string `env:"PROMETHEUS_ADDRESS" required:"true"`
	TraceCollector    string `env:"TRACE_COLLECTOR" required:"true"`

	ClientInterval time.Duration `env:"CLIENT_INTERVAL" envDefault:"1s"`
}

func New() (Config, error) {
	var cfg Config
	err := env.Parse(&cfg)
	if err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}

	return cfg, nil
}
