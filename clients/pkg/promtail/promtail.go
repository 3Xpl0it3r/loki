package promtail

import (
	"sync"

	"github.com/go-kit/log/level"
	"github.com/grafana/loki/clients/pkg/promtail/client/metrics"
	cri "github.com/grafana/loki/clients/pkg/promtail/container"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/loki/clients/pkg/logentry/stages"
	"github.com/grafana/loki/clients/pkg/promtail/client"
	"github.com/grafana/loki/clients/pkg/promtail/config"
	"github.com/grafana/loki/clients/pkg/promtail/server"
	"github.com/grafana/loki/clients/pkg/promtail/targets"
	"github.com/grafana/loki/clients/pkg/promtail/targets/target"

	util_log "github.com/grafana/loki/pkg/util/log"
)

// Option is a function that can be passed to the New method of Promtail and
// customize the Promtail that is created.
type Option func(p *Promtail)

// WithLogger overrides the default logger for Promtail.
func WithLogger(log log.Logger) Option {
	return func(p *Promtail) {
		p.logger = log
	}
}

// WithRegisterer overrides the default registerer for Promtail.
func WithRegisterer(reg prometheus.Registerer) Option {
	return func(p *Promtail) {
		p.reg = reg
	}
}

// Promtail is the root struct for Promtail.
type Promtail struct {
	client         client.Client
	targetManagers *targets.TargetManagers
	server         server.Server
	logger         log.Logger
	reg            prometheus.Registerer

	criClient cri.Client

	stopped bool
	mtx     sync.Mutex
}

// New makes a new Promtail.
func New(cfg config.Config, metrics *metrics.Metrics, dryRun bool, opts ...Option) (*Promtail, error) {
	// Initialize promtail with some defaults and allow the options to override
	// them.
	promtail := &Promtail{
		logger: util_log.Logger,
		reg:    prometheus.DefaultRegisterer,
	}
	for _, o := range opts {
		// todo (callum) I don't understand why I needed to add this check
		if o == nil {
			continue
		}
		o(promtail)
	}

	// create criClient
	criClient, err := cri.NewClient()
	if err != nil {
		level.Error(promtail.logger).Log("Failed Create Promtail", err)
		return nil, err
	}

	if err = criClient.Ping(); err != nil {
		level.Error(promtail.logger).Log("Failed Create Promtail", err)
		return nil, err
	}
	promtail.criClient = criClient

	cfg.Setup(promtail.logger)

	if cfg.LimitsConfig.ReadlineRateEnabled {
		stages.SetReadLineRateLimiter(cfg.LimitsConfig.ReadlineRate, cfg.LimitsConfig.ReadlineBurst, cfg.LimitsConfig.ReadlineRateDrop)
	}

	if dryRun {
		promtail.client, err = client.NewLogger(metrics, cfg.Options.StreamLagLabels, promtail.logger, cfg.ClientConfigs)
		if err != nil {
			return nil, err
		}
		cfg.PositionsConfig.ReadOnly = true
	} else {
		promtail.client, err = client.NewMulti(metrics, cfg.Options.StreamLagLabels, promtail.logger, cfg.ClientConfigs)
		if err != nil {
			return nil, err
		}
	}

	tms, err := targets.NewTargetManagers(promtail, promtail.reg, promtail.logger, cfg.PositionsConfig, promtail.client, cfg.ScrapeConfig, &cfg.TargetConfig, criClient)
	if err != nil {
		return nil, err
	}
	promtail.targetManagers = tms
	server, err := server.New(cfg.ServerConfig, promtail.logger, tms, cfg.String())
	if err != nil {
		return nil, err
	}
	promtail.server = server
	return promtail, nil
}

// Run the promtail; will block until a signal is received.
func (p *Promtail) Run() error {
	p.mtx.Lock()
	// if we stopped promtail before the server even started we can return without starting.
	if p.stopped {
		p.mtx.Unlock()
		return nil
	}
	p.mtx.Unlock() // unlock before blocking
	return p.server.Run()
}

// Client returns the underlying client Promtail uses to write to Loki.
func (p *Promtail) Client() client.Client {
	return p.client
}

// Shutdown the promtail.
func (p *Promtail) Shutdown() {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if p.stopped {
		return
	}
	p.stopped = true
	if p.server != nil {
		p.server.Shutdown()
	}
	if p.targetManagers != nil {
		p.targetManagers.Stop()
	}
	// todo work out the stop.
	p.client.Stop()
    p.criClient.Stop()
}

// ActiveTargets returns active targets per jobs from the target manager
func (p *Promtail) ActiveTargets() map[string][]target.Target {
	return p.targetManagers.ActiveTargets()
}
