package main

import (
	"fmt"
	"github.com/grafana/loki/clients/pkg/promtail/client/metrics"

	"github.com/go-kit/log"

	"github.com/grafana/loki/clients/pkg/promtail/client"
)

type bufferConfig struct {
	buffer     bool
	bufferType string
	dqueConfig dqueConfig
}

var defaultBufferConfig = bufferConfig{
	buffer:     false,
	bufferType: "dque",
	dqueConfig: defaultDqueConfig,
}

// NewBuffer makes a new buffered Client.
func NewBuffer(cfg *config, logger log.Logger, metrics *metrics.Metrics, streamLagLabels []string) (client.Client, error) {
	switch cfg.bufferConfig.bufferType {
	case "dque":
		return newDque(cfg, logger, metrics, streamLagLabels)
	default:
		return nil, fmt.Errorf("failed to parse bufferType: %s", cfg.bufferConfig.bufferType)
	}
}
