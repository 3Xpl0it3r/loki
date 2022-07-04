package metrics

import (
	"fmt"
	"github.com/grafana/loki/pkg/util/build"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	contentType  = "application/x-protobuf"
	maxErrMsgLen = 1024

	// Label reserved to override the tenant ID while processing
	// pipeline stages
	ReservedLabelTenantID = "__tenant_id__"

	LatencyLabel = "filename"
	HostLabel    = "host"
	ClientLabel  = "client"
)

var UserAgent = fmt.Sprintf("promtail/%s", build.Version)

type Metrics struct {
	EncodedBytes     *prometheus.CounterVec
	SentBytes        *prometheus.CounterVec
	DroppedBytes     *prometheus.CounterVec
	SentEntries      *prometheus.CounterVec
	DroppedEntries   *prometheus.CounterVec
	RequestDuration  *prometheus.HistogramVec
	BatchRetries     *prometheus.CounterVec
	CountersWithHost []*prometheus.CounterVec
	StreamLag        *prometheus.GaugeVec
    // this is for debug
    TotalLines  *prometheus.CounterVec
}


func NewMetrics(reg prometheus.Registerer, streamLagLabels []string) *Metrics {
	var m Metrics

	m.EncodedBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "promtail",
		Name:      "encoded_bytes_total",
		Help:      "Number of bytes encoded and ready to send.",
	}, []string{HostLabel})
	m.SentBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "promtail",
		Name:      "sent_bytes_total",
		Help:      "Number of bytes sent.",
	}, []string{HostLabel})
	m.DroppedBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "promtail",
		Name:      "dropped_bytes_total",
		Help:      "Number of bytes dropped because failed to be sent to the ingester after all retries.",
	}, []string{HostLabel})
	m.SentEntries = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "promtail",
		Name:      "sent_entries_total",
		Help:      "Number of log entries sent to the ingester.",
	}, []string{HostLabel})
	m.DroppedEntries = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "promtail",
		Name:      "dropped_entries_total",
		Help:      "Number of log entries dropped because failed to be sent to the ingester after all retries.",
	}, []string{HostLabel})
	m.RequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "promtail",
		Name:      "request_duration_seconds",
		Help:      "Duration of send requests.",
	}, []string{"status_code", HostLabel})
	m.BatchRetries = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "promtail",
		Name:      "batch_retries_total",
		Help:      "Number of times batches has had to be retried.",
	}, []string{HostLabel})

    // add debug metreics
    m.TotalLines = prometheus.NewCounterVec(prometheus.CounterOpts{
        Namespace: "promtail",
        Name: "multiclient_receivelines_total",
        Help: "Multi client receive total lines",
    }, []string{HostLabel})


	m.CountersWithHost = []*prometheus.CounterVec{
		m.EncodedBytes, m.SentBytes, m.DroppedBytes, m.SentEntries, m.DroppedEntries,
	}

	streamLagLabelsMerged := []string{HostLabel, ClientLabel}
	streamLagLabelsMerged = append(streamLagLabelsMerged, streamLagLabels...)
	m.StreamLag = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "promtail",
		Name:      "stream_lag_seconds",
		Help:      "Difference between current time and last batch timestamp for successful sends",
	}, streamLagLabelsMerged)

	if reg != nil {
		m.EncodedBytes = mustRegisterOrGet(reg, m.EncodedBytes).(*prometheus.CounterVec)
		m.SentBytes = mustRegisterOrGet(reg, m.SentBytes).(*prometheus.CounterVec)
		m.DroppedBytes = mustRegisterOrGet(reg, m.DroppedBytes).(*prometheus.CounterVec)
		m.SentEntries = mustRegisterOrGet(reg, m.SentEntries).(*prometheus.CounterVec)
		m.DroppedEntries = mustRegisterOrGet(reg, m.DroppedEntries).(*prometheus.CounterVec)
		m.RequestDuration = mustRegisterOrGet(reg, m.RequestDuration).(*prometheus.HistogramVec)
		m.BatchRetries = mustRegisterOrGet(reg, m.BatchRetries).(*prometheus.CounterVec)
		m.StreamLag = mustRegisterOrGet(reg, m.StreamLag).(*prometheus.GaugeVec)
	}

	return &m
}

func mustRegisterOrGet(reg prometheus.Registerer, c prometheus.Collector) prometheus.Collector {
	if err := reg.Register(c); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			return are.ExistingCollector
		}
		panic(err)
	}
	return c
}

