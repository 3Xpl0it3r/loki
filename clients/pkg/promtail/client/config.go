package client

import (
	"flag"
	"github.com/go-kit/log"
	"github.com/grafana/loki/clients/pkg/promtail/api"
	"github.com/grafana/loki/clients/pkg/promtail/client/filesystem"
	"github.com/grafana/loki/clients/pkg/promtail/client/kafka"
	"github.com/grafana/loki/clients/pkg/promtail/client/loki"
	clientmetrics "github.com/grafana/loki/clients/pkg/promtail/client/metrics"
	"github.com/pkg/errors"
)

type Client interface {
	api.EntryHandler
	// Stop goroutine sending batch of entries without retries.
	StopNow()
	Name()string
}

type ClientKind string

const (
	KafkaClient ClientKind = "kafka"
	LokiClient  ClientKind = "loki"
	FileClient  ClientKind = "file-system"
)

type RunnerAble interface {
	api.EntryHandler
	// Stop goroutine sending batch of entries without retries.
	StopNow()
}

// NOTE the helm chart for promtail and fluent-bit also have defaults for these values, please update to match if you make changes here.

// Config describes configuration for a HTTP pusher client.
type Configs struct {
	// lokiconfig
	LokiConfig loki.LokiConfig `yaml:"loki,omitempty"`
	// kafka
	KafkaConfig kafka.KafkaConfig `yaml:"kafka,omitempty"`
	// file system config
	FileSystemClient filesystem.FileClientConfig `yaml:"file_system_config,omitempty"`
}

// RegisterFlags with prefix registers flags where every name is prefixed by
// prefix. If prefix is a non-empty string, prefix should end with a period.
func (c *Configs) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	if c.LokiConfig.URL.URL != nil {
		c.LokiConfig.RegisterFlagsWithPrefix(prefix, f)
	} else if c.FileSystemClient.Path != "" {
		c.FileSystemClient.RegisterFlagsWithPrefix(prefix, f)
	}

}

// RegisterFlags registers flags.
func (c *Configs) RegisterFlags(flags *flag.FlagSet) {
	if c.LokiConfig.URL.URL != nil {
		c.LokiConfig.RegisterFlags(flags)
	} else if c.FileSystemClient.Path != "" {
		c.FileSystemClient.RegisterFlags(flags)
	}

}

// UnmarshalYAML implement Yaml Unmarshaler
func (c *Configs) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type raw Configs

	var cfg raw

	cfg = raw{
		LokiConfig:       loki.DefaultLokiConfig(),
		KafkaConfig:      kafka.DefaultKafkaConfig(),
		FileSystemClient: filesystem.DefaultFileSystemConfig(),
	}


	if err := unmarshal(&cfg); err != nil {
		return err
	}

	*c = Configs(cfg)
	return nil
}
//
// NewClientFromConfig return the an client according to the config
func (c *Configs) NewClientFromConfig(clientKind ClientKind,metrics *clientmetrics.Metrics, streamLagLabels []string,logger log.Logger) (Client, error) {
	switch clientKind {
	case LokiClient:
		return loki.New(metrics, c.LokiConfig, streamLagLabels, logger)
	case FileClient:
		return filesystem.NewFileSystemClient(metrics, c.FileSystemClient, streamLagLabels, logger)
	case KafkaClient:
		return kafka.NewKafkaClient(metrics, c.KafkaConfig, streamLagLabels, logger)
	default:
		return nil, errors.New("unknown types of client")
	}
}



