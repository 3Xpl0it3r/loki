package client

import (
	"errors"
	util_log "github.com/cortexproject/cortex/pkg/util/log"
	"github.com/go-kit/kit/log/level"
	"github.com/grafana/loki/clients/pkg/promtail/client/config"
	"sync"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/loki/clients/pkg/promtail/api"

	"github.com/grafana/loki/pkg/util/flagext"
)




// MultiClient is client pushing to one or more loki instances.
type MultiClient struct {
	clients []config.Client
	entries chan api.Entry
	wg      sync.WaitGroup

	once sync.Once
}

// NewMulti creates a new client
func NewMulti(reg prometheus.Registerer, logger log.Logger, externalLabels flagext.LabelSet, cfgs ...config.Config) (config.Client, error) {
	if len(cfgs) == 0 {
		return nil, errors.New("at least one client config should be provided")
	}

	clients := make([]config.Client, 0, len(cfgs))

	for _, cfg := range cfgs {

		// Merge the provided external labels from the single client config/command line with each client config from
		// `clients`. This is done to allow --client.external-labels=key=value passed at command line to apply to all clients
		// The order here is specified to allow the yaml to override the command line flag if there are any labels
		// which exist in both the command line arguments as well as the yaml, and while this is
		// not typically the order of precedence, the assumption here is someone providing a specific config in
		// yaml is doing so explicitly to make a key specific to a client.
		client,err := cfg.NewClientFromConfig(reg, logger)
		if err != nil{
			level.Error(util_log.Logger).Log("msg", "failed to create client", "err", err.Error())
		}
		//client, err := loki.New(reg, cfg.LokiConfig, logger)
		//if err != nil {
		//	return nil, err
		//}
		clients = append(clients, client)

	}
	multi := &MultiClient{
		clients: clients,
		entries: make(chan api.Entry),
	}
	multi.start()
	return multi, nil
}

func (m *MultiClient) start() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for e := range m.entries {
			for _, c := range m.clients {
				c.Chan() <- e
			}
		}
	}()
}

func (m *MultiClient) Chan() chan<- api.Entry {
	return m.entries
}

// Stop implements Client
func (m *MultiClient) Stop() {
	m.once.Do(func() { close(m.entries) })
	m.wg.Wait()
	for _, c := range m.clients {
		c.Stop()
	}
}

// StopNow implements Client
func (m *MultiClient) StopNow() {
	for _, c := range m.clients {
		c.StopNow()
	}
}
