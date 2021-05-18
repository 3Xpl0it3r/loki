package filesystem

import (
	"flag"
	"time"

	"github.com/cortexproject/cortex/pkg/util"
	lokiflag "github.com/grafana/loki/pkg/util/flagext"
)

const (
	BatchWait      = 1 * time.Second
	BatchSize  int = 1024 * 100
	Timeout        = 10 * time.Second
)

type FileClientConfig struct {
	Path string `json:"path"`
	Rolling int `json:"rolling"`
	BatchWait time.Duration
	BatchSize int
	BackoffConfig util.BackoffConfig `yaml:"backoff_config"`
	// The labels to add to any time series or alerts when communicating with loki
	ExternalLabels lokiflag.LabelSet `yaml:"external_labels,omitempty"`
	Timeout        time.Duration     `yaml:"timeout"`
}


func (c *FileClientConfig) RegisterFlagsWithPrefix(prefix string, f *flag.FlagSet) {
	// do nothing
	return
}

func(c *FileClientConfig)RegisterFlags(flags *flag.FlagSet){

}

func DefaultFileSystemConfig()FileClientConfig{
	return FileClientConfig{
		BatchSize: BatchSize,
		BatchWait: BatchWait,
		Timeout:   Timeout,
	}
}
