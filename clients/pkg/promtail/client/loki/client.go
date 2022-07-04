package loki

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/grafana/dskit/backoff"
	"github.com/grafana/loki/clients/pkg/promtail/client/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/prometheus/promql/parser"

	"github.com/grafana/loki/clients/pkg/promtail/api"

	lokiutil "github.com/grafana/loki/pkg/util"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
)

const (
	contentType  = "application/x-protobuf"
	maxErrMsgLen = 1024

	// Label reserved to override the tenant ID while processing
	// pipeline stages
	ReservedLabelTenantID = "__tenant_id__"

	LatencyLabel = "filename"
	HostLabel    = "host"
	ClientLabel = "client"
)

const (
	ControllerNameLabel model.LabelName = "controller_name"
)

var UserAgent = fmt.Sprintf("promtail/%s", version.Version)

// Client pushes entries to Loki and can be stopped

// Client for pushing logs in snappy-compressed protos over HTTP.
type client struct {
	metrics *metrics.Metrics
	streamLagLabels []string
	logger  log.Logger
	cfg     LokiConfig
	client  *http.Client
	entries chan api.Entry

	once sync.Once
	wg   sync.WaitGroup

	externalLabels model.LabelSet

	// ctx is used in any upstream calls from the `client`.
	ctx    context.Context
	cancel context.CancelFunc
}

// New makes a new Client.
func New(metrics *metrics.Metrics, cfg LokiConfig, streamLagLabels []string,logger log.Logger) (*client, error) {
	if cfg.URL.URL == nil {
		return nil, errors.New("client needs target URL")
	}
	ctx, cancel := context.WithCancel(context.Background())

	c := &client{
		logger:  log.With(logger, "component", "client", "host", cfg.URL.Host),
		cfg:     cfg,
		streamLagLabels: streamLagLabels,
		entries: make(chan api.Entry),
		metrics: metrics,

		externalLabels: cfg.ExternalLabels.LabelSet,
		ctx:            ctx,
		cancel:         cancel,
	}

	err := cfg.Client.Validate()
	if err != nil {
		return nil, err
	}

	c.client, err = config.NewClientFromConfig(cfg.Client, "promtail")
	if err != nil {
		return nil, err
	}

	c.client.Timeout = cfg.Timeout

	// Initialize counters to 0 so the metrics are exported before the first
	// occurrence of incrementing to avoid missing metrics.
	for _, counter := range c.metrics.CountersWithHost {
		counter.WithLabelValues(c.cfg.URL.Host).Add(0)
	}

	c.wg.Add(1)
	go c.run()
	return c, nil
}

func (c *client) run() {
	level.Info(c.logger).Log("msg", "loki client start running ....")
	batches := map[string]*batch{}

	// Given the client handles multiple batches (1 per tenant) and each batch
	// can be created at a different point in time, we look for batches whose
	// max wait time has been reached every 10 times per BatchWait, so that the
	// maximum delay we have sending batches is 10% of the max waiting time.
	// We apply a cap of 10ms to the ticker, to avoid too frequent checks in
	// case the BatchWait is very low.
	minWaitCheckFrequency := 10 * time.Millisecond
	maxWaitCheckFrequency := c.cfg.BatchWait / 10
	if maxWaitCheckFrequency < minWaitCheckFrequency {
		maxWaitCheckFrequency = minWaitCheckFrequency
	}

	maxWaitCheck := time.NewTicker(maxWaitCheckFrequency)

	defer func() {
		maxWaitCheck.Stop()
		// Send all pending batches
		for tenantID, batch := range batches {
			c.sendBatch(tenantID, batch)
		}

		c.wg.Done()
	}()

	for {
		select {
		case e, ok := <-c.entries:
			// 获取到条目
			if !ok {
				return
			}

			e, tenantID := c.processEntry(e)
			batch, ok := batches[tenantID]

			// If the batch doesn't exist yet, we create a new one with the entry
			if !ok {
				batches[tenantID] = newBatch(e)
				break
			}

			// If adding the entry to the batch will increase the size over the max
			// size allowed, we do send the current batch and then create a new one
			if batch.sizeBytesAfter(e) > c.cfg.BatchSize {
				c.sendBatch(tenantID, batch)

				batches[tenantID] = newBatch(e)
				break
			}

			// The max size of the batch isn't reached, so we can add the entry
			batch.add(e)

		case <-maxWaitCheck.C:
			// Send all batches whose max wait time has been reached
			for tenantID, batch := range batches {
				if batch.age() < c.cfg.BatchWait {
					continue
				}

				c.sendBatch(tenantID, batch)
				delete(batches, tenantID)
			}
		}
	}
}

// valid add filter when push log to loki
// if return true ,the entry is valid ,then can be push to loki
// if return false ,then entry should be dropped
func (c *client) valid(entry *api.Entry) bool {
	if dplName, ok := entry.Labels[ControllerNameLabel]; !ok {
		return false
	} else {
		return strings.Contains(string(dplName), "workflow")
	}

}

func (c *client) Chan() chan<- api.Entry {
	return c.entries
}

func (c *client) sendBatch(tenantID string, batch *batch) {
	// todo
	buf, entriesCount, err := batch.encode()
	if err != nil {
		level.Error(c.logger).Log("msg", "error encoding batch", "error", err)
		return
	}
	bufBytes := float64(len(buf))
	c.metrics.EncodedBytes.WithLabelValues(c.cfg.URL.Host).Add(bufBytes)

	bfretry := backoff.New(c.ctx, c.cfg.BackoffConfig)
	var status int
	for {
		start := time.Now()
		// send uses `timeout` internally, so `context.Background` is good enough.
		status, err = c.send(context.Background(), tenantID, buf)

		c.metrics.RequestDuration.WithLabelValues(strconv.Itoa(status), c.cfg.URL.Host).Observe(time.Since(start).Seconds())

		if err == nil {
			c.metrics.SentBytes.WithLabelValues(c.cfg.URL.Host).Add(bufBytes)
			c.metrics.SentEntries.WithLabelValues(c.cfg.URL.Host).Add(float64(entriesCount))
			for _, s := range batch.streams {
				lbls, err := parser.ParseMetric(s.Labels)
				if err != nil {
					// is this possible?
					level.Warn(c.logger).Log("msg", "error converting stream label string to label.Labels, cannot update lagging metric", "error", err)
					return
				}
				var lblSet = make(prometheus.Labels)
				for _,lbl := range c.streamLagLabels{
					// labels from streamLaglabels may not be found but we still need empty value
					value := ""
					for i := range lbls{
						if lbls[i].Name == lbl {
							value = lbls[i].Name
						}
						lblSet[lbl] = value
					}
				}

				if lblSet != nil {
					lblSet[HostLabel] = c.cfg.URL.Host
					lblSet[ClientLabel] = "loki"
					c.metrics.StreamLag.With(lblSet).Set(time.Since(s.Entries[len(s.Entries)-1].Timestamp).Seconds())
				}
			}
			return
		}

		// Only retry 429s, 500s and connection-level errors.
		if status > 0 && status != 429 && status/100 != 5 {
			break
		}

		level.Warn(c.logger).Log("msg", "error sending batch, will retry", "status", status, "error", err)
		c.metrics.BatchRetries.WithLabelValues(c.cfg.URL.Host).Inc()
		bfretry.Wait()

		// Make sure it sends at least once before checking for retry.
		if !bfretry.Ongoing() {
			break
		}
	}

	if err != nil {
		level.Error(c.logger).Log("msg", "final error sending batch", "status", status, "error", err)
		c.metrics.DroppedBytes.WithLabelValues(c.cfg.URL.Host).Add(bufBytes)
		c.metrics.DroppedEntries.WithLabelValues(c.cfg.URL.Host).Add(float64(entriesCount))
	}
}

func (c *client) send(ctx context.Context, tenantID string, buf []byte) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	req, err := http.NewRequest("POST", c.cfg.URL.String(), bytes.NewReader(buf))
	if err != nil {
		return -1, err
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", UserAgent)

	// If the tenant ID is not empty promtail is running in multi-tenant mode, so
	// we should send it to Loki
	if tenantID != "" {
		req.Header.Set("X-Scope-OrgID", tenantID)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return -1, err
	}
	defer lokiutil.LogError("closing response body", resp.Body.Close)

	if resp.StatusCode/100 != 2 {
		scanner := bufio.NewScanner(io.LimitReader(resp.Body, maxErrMsgLen))
		line := ""
		if scanner.Scan() {
			line = scanner.Text()
		}
		err = fmt.Errorf("server returned HTTP status %s (%d): %s", resp.Status, resp.StatusCode, line)
	}
	return resp.StatusCode, err
}

func (c *client) getTenantID(labels model.LabelSet) string {
	// Check if it has been overridden while processing the pipeline stages
	if value, ok := labels[ReservedLabelTenantID]; ok {
		return string(value)
	}

	// Check if has been specified in the config
	if c.cfg.TenantID != "" {
		return c.cfg.TenantID
	}

	// Defaults to an empty string, which means the X-Scope-OrgID header
	// will not be sent
	return ""
}

// Stop the client.
func (c *client) Stop() {
	c.once.Do(func() { close(c.entries) })
	c.wg.Wait()
}

// StopNow stops the client without retries
func (c *client) StopNow() {
	// cancel will stop retrying http requests.
	c.cancel()
	c.Stop()
}

func (c *client) processEntry(e api.Entry) (api.Entry, string) {
	if len(c.externalLabels) > 0 {
		e.Labels = c.externalLabels.Merge(e.Labels)
	}
	var custom model.LabelSet = map[model.LabelName]model.LabelValue{model.LabelName("test"): model.LabelValue("hh")}
	e.Labels = custom.Merge(e.Labels)
	tenantID := c.getTenantID(e.Labels)
	return e, tenantID
}

func (c *client) UnregisterLatencyMetric(labels prometheus.Labels) {
	labels[HostLabel] = c.cfg.URL.Host
	c.metrics.StreamLag.Delete(labels)
}

func (c *client) Name() string {
	// this should be returned the url of client
	return "loki"
}
