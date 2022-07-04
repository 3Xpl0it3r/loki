package kafka

import (
	"time"

	"github.com/grafana/dskit/backoff"
	lokiflag "github.com/grafana/loki/pkg/util/flagext"
)

const (
	BatchWait      = 1 * time.Second
	BatchSize  int = 1 * 1024 * 1024
	MinBackoff     = 500 * time.Millisecond
	MaxBackoff     = 1 * time.Minute
	MaxRetries int = 10
	Timeout        = 10 * time.Second

	ProducerMaxMessageSize int = 1 * 1024 * 1024
)

type KafkaConfig struct {
	// dia of kakfa that client want to connect
	Url       string `yaml:"url"`
	Protocol  string `yaml:"protocol"`
	Topic     string `yaml:"topic"`
	GroupId   string `yaml:"groupId"`
	Partition int    `yaml:"partition"`

	BatchWait time.Duration `yaml:"batch_wait"`
	// The max number of entries  bytes can be cached in batch buffer
	BatchSize     int            `yaml:"batch_size"`
	BackoffConfig backoff.Config `yaml:"backoff_config"`
	// The labels to add to any time series or alerts when communicating with loki
	ExternalLabels lokiflag.LabelSet `yaml:"external_labels,omitempty"`
	Timeout        time.Duration     `yaml:"timeout"`

	// The max number of message bytes that can be allowed to send to kafka server
	ProducerMaxMessageSize int `yaml:"producer_max_message_size"`
}

func DefaultKafkaConfig() KafkaConfig {
	return KafkaConfig{
		Protocol:  "tcp",
		Topic:     "example_topic",
		GroupId:   "",
		Partition: 0,
		BatchWait: BatchWait,
		BatchSize: BatchSize,
		BackoffConfig: backoff.Config{
			MaxBackoff: MaxBackoff,
			MinBackoff: MinBackoff,
			MaxRetries: MaxRetries,
		},
		ExternalLabels:         lokiflag.LabelSet{},
		Timeout:                Timeout,
		ProducerMaxMessageSize: ProducerMaxMessageSize,
	}
}
