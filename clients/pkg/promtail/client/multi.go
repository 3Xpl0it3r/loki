package client

import (
	"strings"
	"sync"

	"github.com/go-kit/log"

	"github.com/grafana/loki/clients/pkg/promtail/api"
	clientmetrics "github.com/grafana/loki/clients/pkg/promtail/client/metrics"
)

const MAX_CLIENT = 3

// MultiClient is client pushing to one or more loki instances.
type MultiClient struct {
	clients []Client
	entries chan api.Entry
	wg      sync.WaitGroup

    // this metrics is used to debug ,this should be removed in future
    metrics *clientmetrics.Metrics

	once sync.Once
}

// NewMulti creates a new client
func NewMulti(metrics *clientmetrics.Metrics, streamLagLabels []string, logger log.Logger, cfgs Configs) (Client, error) {
	var fake struct{}
	clientsCheck := make(map[string]struct{})
	clients:= make([]Client, 0, MAX_CLIENT)
	if cfgs.LokiConfig.URL.URL != nil{
		client,err := cfgs.NewClientFromConfig(LokiClient,metrics, streamLagLabels, logger)
		if err  != nil {
			return nil, err
		}
		clientsCheck[client.Name()] = fake
		clients = append(clients, client)
	}
	if cfgs.FileSystemClient.Path != ""{
		client,err := cfgs.NewClientFromConfig(FileClient, metrics, streamLagLabels, logger)
		if err  != nil {
			return nil, err
		}
		clientsCheck[client.Name()] = fake
		clients = append(clients, client)
	}
	if cfgs.KafkaConfig.Url != ""{
		client,err := cfgs.NewClientFromConfig(KafkaClient, metrics, streamLagLabels, logger)
		if err  != nil {
			return nil, err
		}
		clientsCheck[client.Name()] = fake
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

func (m *MultiClient) Name() string {
	var sb strings.Builder
	sb.WriteString("multi:")
	for i, c := range m.clients {
		sb.WriteString(c.Name())
		if i != len(m.clients)-1 {
			sb.WriteString(",")
		}
	}
	return sb.String()
}
