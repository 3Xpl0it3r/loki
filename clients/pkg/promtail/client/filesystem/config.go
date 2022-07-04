package filesystem

import (
	"flag"

	"time"

	"github.com/grafana/dskit/backoff"
	lokiflag "github.com/grafana/loki/pkg/util/flagext"

)

const (
	DefaultBatchWait time.Duration = 1 * time.Second
	DefaultBatchSize int           = 1024 * 100
	DefaultTimeout   time.Duration = 10 * time.Second
)

type FileClientConfig struct {
	Path             string `json:"path"`
	Rolling          int    `json:"rolling"`
	BatchWait        time.Duration
	BatchSize int
	BackoffConfig    backoff.Config `yaml:"backoff_config"`
	// The labels to add to any time series or alerts when communicating with loki
	ExternalLabels lokiflag.LabelSet `yaml:"external_labels,omitempty"`
	Timeout        time.Duration     `yaml:"timeout"`
	ReSyncPeriod   time.Duration     `yaml:"resync_period"`
}

func (c *FileClientConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	// do nothing
	return
}

func (c *FileClientConfig) RegisterFlags(flags *flag.FlagSet) {

}

func DefaultFileSystemConfig() FileClientConfig {
	return FileClientConfig{
		BatchSize: DefaultBatchSize,
		BatchWait:        DefaultBatchWait,
		Timeout:          DefaultTimeout,
		ReSyncPeriod:     DefaultFileHandlerReSyncPeriod,
	}
}
