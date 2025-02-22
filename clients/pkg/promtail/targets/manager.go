package targets

import (
	"fmt"
	cri "github.com/grafana/loki/clients/pkg/promtail/container"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/loki/clients/pkg/promtail/api"
	"github.com/grafana/loki/clients/pkg/promtail/positions"
	"github.com/grafana/loki/clients/pkg/promtail/scrapeconfig"
	"github.com/grafana/loki/clients/pkg/promtail/targets/cloudflare"
	"github.com/grafana/loki/clients/pkg/promtail/targets/docker"
	"github.com/grafana/loki/clients/pkg/promtail/targets/file"
	"github.com/grafana/loki/clients/pkg/promtail/targets/gcplog"
	"github.com/grafana/loki/clients/pkg/promtail/targets/gelf"
	"github.com/grafana/loki/clients/pkg/promtail/targets/journal"
	"github.com/grafana/loki/clients/pkg/promtail/targets/kafka"
	"github.com/grafana/loki/clients/pkg/promtail/targets/lokipush"
	"github.com/grafana/loki/clients/pkg/promtail/targets/stdin"
	"github.com/grafana/loki/clients/pkg/promtail/targets/syslog"
	"github.com/grafana/loki/clients/pkg/promtail/targets/target"
	"github.com/grafana/loki/clients/pkg/promtail/targets/windows"
)

const (
	FileScrapeConfigs    = "fileScrapeConfigs"
	JournalScrapeConfigs = "journalScrapeConfigs"
	SyslogScrapeConfigs  = "syslogScrapeConfigs"
	GcplogScrapeConfigs  = "gcplogScrapeConfigs"
	PushScrapeConfigs    = "pushScrapeConfigs"
	WindowsEventsConfigs = "windowsEventsConfigs"
	KafkaConfigs         = "kafkaConfigs"
	GelfConfigs          = "gelfConfigs"
	CloudflareConfigs    = "cloudflareConfigs"
	DockerConfigs        = "dockerConfigs"
	DockerSDConfigs      = "dockerSDConfigs"
)

type targetManager interface {
	Ready() bool
	Stop()
	ActiveTargets() map[string][]target.Target
	AllTargets() map[string][]target.Target
}

// TargetManagers manages a list of target managers.
type TargetManagers struct {
	targetManagers []targetManager
	positions      positions.Positions
}

// NewTargetManagers makes a new TargetManagers
func NewTargetManagers(
	app stdin.Shutdownable,
	reg prometheus.Registerer,
	logger log.Logger,
	positionsConfig positions.Config,
	client api.EntryHandler,
	scrapeConfigs []scrapeconfig.Config,
	targetConfig *file.Config,
	criClient cri.Client,
) (*TargetManagers, error) {
	if targetConfig.Stdin {
		level.Debug(logger).Log("msg", "configured to read from stdin")
		stdin, err := stdin.NewStdinTargetManager(reg, logger, app, client, scrapeConfigs)
		if err != nil {
			return nil, err
		}
		return &TargetManagers{targetManagers: []targetManager{stdin}}, nil
	}

	var targetManagers []targetManager
	targetScrapeConfigs := make(map[string][]scrapeconfig.Config, 4)

	for _, cfg := range scrapeConfigs {
		switch {
		case cfg.HasServiceDiscoveryConfig():
			targetScrapeConfigs[FileScrapeConfigs] = append(targetScrapeConfigs[FileScrapeConfigs], cfg)
		case cfg.JournalConfig != nil:
			targetScrapeConfigs[JournalScrapeConfigs] = append(targetScrapeConfigs[JournalScrapeConfigs], cfg)
		case cfg.SyslogConfig != nil:
			targetScrapeConfigs[SyslogScrapeConfigs] = append(targetScrapeConfigs[SyslogScrapeConfigs], cfg)
		case cfg.GcplogConfig != nil:
			targetScrapeConfigs[GcplogScrapeConfigs] = append(targetScrapeConfigs[GcplogScrapeConfigs], cfg)
		case cfg.PushConfig != nil:
			targetScrapeConfigs[PushScrapeConfigs] = append(targetScrapeConfigs[PushScrapeConfigs], cfg)
		case cfg.WindowsConfig != nil:
			targetScrapeConfigs[WindowsEventsConfigs] = append(targetScrapeConfigs[WindowsEventsConfigs], cfg)
		case cfg.KafkaConfig != nil:
			targetScrapeConfigs[KafkaConfigs] = append(targetScrapeConfigs[KafkaConfigs], cfg)
		case cfg.GelfConfig != nil:
			targetScrapeConfigs[GelfConfigs] = append(targetScrapeConfigs[GelfConfigs], cfg)
		case cfg.CloudflareConfig != nil:
			targetScrapeConfigs[CloudflareConfigs] = append(targetScrapeConfigs[CloudflareConfigs], cfg)
		case cfg.DockerSDConfigs != nil:
			targetScrapeConfigs[DockerSDConfigs] = append(targetScrapeConfigs[DockerSDConfigs], cfg)
		default:
			return nil, fmt.Errorf("no valid target scrape config defined for %q", cfg.JobName)
		}
	}

	var positionFile positions.Positions

	// position file is a singleton, we use a function to keep it so.
	getPositionFile := func() (positions.Positions, error) {
		if positionFile == nil {
			var err error
			positionFile, err = positions.New(logger, positionsConfig)
			if err != nil {
				return nil, err
			}
		}
		return positionFile, nil
	}

	var (
		fileMetrics       *file.Metrics
		syslogMetrics     *syslog.Metrics
		gcplogMetrics     *gcplog.Metrics
		gelfMetrics       *gelf.Metrics
		cloudflareMetrics *cloudflare.Metrics
		dockerMetrics     *docker.Metrics
		journalMetrics    *journal.Metrics
	)
	if len(targetScrapeConfigs[FileScrapeConfigs]) > 0 {
		fileMetrics = file.NewMetrics(reg)
	}
	if len(targetScrapeConfigs[SyslogScrapeConfigs]) > 0 {
		syslogMetrics = syslog.NewMetrics(reg)
	}
	if len(targetScrapeConfigs[GcplogScrapeConfigs]) > 0 {
		gcplogMetrics = gcplog.NewMetrics(reg)
	}
	if len(targetScrapeConfigs[GelfConfigs]) > 0 {
		gelfMetrics = gelf.NewMetrics(reg)
	}
	if len(targetScrapeConfigs[CloudflareConfigs]) > 0 {
		cloudflareMetrics = cloudflare.NewMetrics(reg)
	}
	if len(targetScrapeConfigs[DockerConfigs]) > 0 || len(targetScrapeConfigs[DockerSDConfigs]) > 0 {
		dockerMetrics = docker.NewMetrics(reg)
	}
	if len(targetScrapeConfigs[JournalScrapeConfigs]) > 0 {
		journalMetrics = journal.NewMetrics(reg)
	}

	for target, scrapeConfigs := range targetScrapeConfigs {
		switch target {
		case FileScrapeConfigs:
			pos, err := getPositionFile()
			if err != nil {
				return nil, err
			}
			fileTargetManager, err := file.NewFileTargetManager(
				fileMetrics,
				logger,
				pos,
				client,
				scrapeConfigs,
				targetConfig,
				criClient,
			)
			if err != nil {
				return nil, errors.Wrap(err, "failed to make file target manager")
			}
			targetManagers = append(targetManagers, fileTargetManager)
		case JournalScrapeConfigs:
			pos, err := getPositionFile()
			if err != nil {
				return nil, err
			}
			journalTargetManager, err := journal.NewJournalTargetManager(
				journalMetrics,
				logger,
				pos,
				client,
				scrapeConfigs,
			)
			if err != nil {
				return nil, errors.Wrap(err, "failed to make journal target manager")
			}
			targetManagers = append(targetManagers, journalTargetManager)
		case SyslogScrapeConfigs:
			syslogTargetManager, err := syslog.NewSyslogTargetManager(
				syslogMetrics,
				logger,
				client,
				scrapeConfigs,
			)
			if err != nil {
				return nil, errors.Wrap(err, "failed to make syslog target manager")
			}
			targetManagers = append(targetManagers, syslogTargetManager)
		case GcplogScrapeConfigs:
			pubsubTargetManager, err := gcplog.NewGcplogTargetManager(
				gcplogMetrics,
				logger,
				client,
				scrapeConfigs,
			)
			if err != nil {
				return nil, errors.Wrap(err, "failed to make syslog target manager")
			}
			targetManagers = append(targetManagers, pubsubTargetManager)
		case PushScrapeConfigs:
			pushTargetManager, err := lokipush.NewPushTargetManager(
				reg,
				logger,
				client,
				scrapeConfigs,
			)
			if err != nil {
				return nil, errors.Wrap(err, "failed to make Loki Push API target manager")
			}
			targetManagers = append(targetManagers, pushTargetManager)
		case WindowsEventsConfigs:
			windowsTargetManager, err := windows.NewTargetManager(reg, logger, client, scrapeConfigs)
			if err != nil {
				return nil, errors.Wrap(err, "failed to make windows target manager")
			}
			targetManagers = append(targetManagers, windowsTargetManager)
		case KafkaConfigs:
			kafkaTargetManager, err := kafka.NewTargetManager(reg, logger, client, scrapeConfigs)
			if err != nil {
				return nil, errors.Wrap(err, "failed to make kafka target manager")
			}
			targetManagers = append(targetManagers, kafkaTargetManager)
		case GelfConfigs:
			gelfTargetManager, err := gelf.NewTargetManager(gelfMetrics, logger, client, scrapeConfigs)
			if err != nil {
				return nil, errors.Wrap(err, "failed to make gelf target manager")
			}
			targetManagers = append(targetManagers, gelfTargetManager)
		case CloudflareConfigs:
			pos, err := getPositionFile()
			if err != nil {
				return nil, err
			}
			cfTargetManager, err := cloudflare.NewTargetManager(cloudflareMetrics, logger, pos, client, scrapeConfigs)
			if err != nil {
				return nil, errors.Wrap(err, "failed to make cloudflare target manager")
			}
			targetManagers = append(targetManagers, cfTargetManager)
		case DockerConfigs:
			pos, err := getPositionFile()
			if err != nil {
				return nil, err
			}
			cfTargetManager, err := docker.NewTargetManager(dockerMetrics, logger, pos, client, scrapeConfigs)
			if err != nil {
				return nil, errors.Wrap(err, "failed to make Docker target manager")
			}
			targetManagers = append(targetManagers, cfTargetManager)
		case DockerSDConfigs:
			pos, err := getPositionFile()
			if err != nil {
				return nil, err
			}
			cfTargetManager, err := docker.NewTargetManager(dockerMetrics, logger, pos, client, scrapeConfigs)
			if err != nil {
				return nil, errors.Wrap(err, "failed to make Docker service discovery target manager")
			}
			targetManagers = append(targetManagers, cfTargetManager)
		default:
			return nil, errors.New("unknown scrape config")
		}
	}

	return &TargetManagers{
		targetManagers: targetManagers,
		positions:      positionFile,
	}, nil
}

// ActiveTargets returns active targets per jobs
func (tm *TargetManagers) ActiveTargets() map[string][]target.Target {
	result := map[string][]target.Target{}
	for _, t := range tm.targetManagers {
		for job, targets := range t.ActiveTargets() {
			result[job] = append(result[job], targets...)
		}
	}
	return result
}

// AllTargets returns all targets per jobs
func (tm *TargetManagers) AllTargets() map[string][]target.Target {
	result := map[string][]target.Target{}
	for _, t := range tm.targetManagers {
		for job, targets := range t.AllTargets() {
			result[job] = append(result[job], targets...)
		}
	}
	return result
}

// Ready if there's at least one ready target manager.
func (tm *TargetManagers) Ready() bool {
	for _, t := range tm.targetManagers {
		if t.Ready() {
			return true
		}
	}
	return false
}

// Stop the TargetManagers.
func (tm *TargetManagers) Stop() {
	for _, t := range tm.targetManagers {
		t.Stop()
	}
	if tm.positions != nil {
		tm.positions.Stop()
	}
}
