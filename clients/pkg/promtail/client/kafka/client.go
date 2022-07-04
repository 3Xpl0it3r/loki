package kafka

import (
	"context"
	"path"
	"strings"
	"sync"
	"time"

	kafka "github.com/Shopify/sarama"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/grafana/loki/clients/pkg/promtail/api"
	clientmetrics "github.com/grafana/loki/clients/pkg/promtail/client/metrics"
)

const (
	contentType  = "application/x-protobuf"
	maxErrMsgLen = 1024
	// Label reserved to override the tenant ID while processing
	// pipeline stages
	ReservedLabelTenantID = "__tenant_id__"
	FilenameLabel         = "filename"
	ddada
	HostLabel = "host"
)

type client struct {
	cfg     KafkaConfig
	entries chan api.Entry
	// add metrics
	metrics *clientmetrics.Metrics

	//kafkaConn *kafka.Conn
	kafkaProducer   kafka.SyncProducer
	streamLagLabels []string
	//kafkaConnPool map[TopicKind]*kafka.Conn

	logger     log.Logger
	once       sync.Once
	wg         sync.WaitGroup
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// New return an elasticsearch client instance
func NewKafkaClient(metrics *clientmetrics.Metrics, cfg KafkaConfig, streamLagLabels []string, logger log.Logger) (*client, error) {
	c := &client{
		cfg:             cfg,
		streamLagLabels: streamLagLabels,
		logger:          logger,
		entries:         make(chan api.Entry),
		metrics:         metrics,
	}
	c.ctx, c.cancelFunc = context.WithCancel(context.Background())

	var err error
	if c.kafkaProducer, err = newKafkaProducer(&cfg); err != nil {
		level.Error(c.logger).Log("msg", "create kafka producer failed", "err", err.Error())
		return nil, err
	}

	c.wg.Add(1)
	go c.run()
	return c, nil
}

func newKafkaProducer(cfg *KafkaConfig) (kafka.SyncProducer, error) {
	config := kafka.NewConfig()
	// 1MB = 1048576  10MB = 10485760
	config.Producer.MaxMessageBytes = cfg.ProducerMaxMessageSize
	config.Producer.Timeout = cfg.Timeout
	config.Producer.Return.Successes = true
	config.Producer.Partitioner = kafka.NewRandomPartitioner
	return kafka.NewSyncProducer([]string{cfg.Url}, config)
}

func (c *client) Chan() chan<- api.Entry {
	return c.entries
}

func (c *client) run() {
	level.Info(c.logger).Log("msg", "kafka client start running....", "batchLimit", c.cfg.BatchSize)
	batches := map[string]*batch{}

	maxWaitCheck := time.NewTicker(5 * time.Second)
	defer func() {
		maxWaitCheck.Stop()
		// Send all pending batches
		for _, batch := range batches {
			c.sendBatch(batch)
		}
		c.wg.Done()
	}()

	for {
		select {
		case e, ok := <-c.entries:
			if !ok {
				return
			}

			if !c.validateEntry(&e) {
				break
			}

			// entry is {{labels map}, lines, timestamp}
			e, tenantId := c.processEntry(e)
			batch, ok := batches[tenantId]

			// If the batch doesn't exist yet, we create a new one with the entry
			if !ok {
				batches[tenantId] = newBatch(e)
				break
			}

			// If adding the entry to the batch will increase the size over the max
			// size allowed, we do send the current batch and then create a new one
			if batch.sizeBytesAfter(e) > c.cfg.BatchSize {
				c.sendBatch(batch)
				batches[tenantId] = newBatch(e)
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
				c.sendBatch(batch)
				delete(batches, tenantID)
			}
		}
	}
}

// Stop the client.
func (c *client) Stop() {
	c.once.Do(func() { close(c.entries) })
	if err := c.kafkaProducer.Close(); err != nil {
		level.Error(c.logger).Log("msg", "close kafka writer failed", "err", err)
	}
	c.wg.Wait()
}

// StopNow stops the client without retries

func (c *client) StopNow() {
	// cancel will stop retrying http requests.
	c.cancelFunc()
	c.Stop()
}

func (c *client) processEntry(e api.Entry) (api.Entry, string) {
	return e, ReservedLabelTenantID
}

func (c *client) sendBatch(batch *batch) {
	requests, _, err := batch.encode()
	defer batch.free()
	if err != nil {
		level.Error(c.logger).Log("kafkaEntry", "error encoding batch", "error", err)
		return
	}

	if status, err := c.send(batch.sizeBytes(), requests); err != nil {
		level.Warn(c.logger).Log("msg", "error sending batch, this batch will be dropped", "status", status, "error", err)
	}
}

func (c *client) send(batchSize int, messages []*kafka.ProducerMessage) (int, error) {
	n := len(messages)

	errs := c.kafkaProducer.SendMessages(messages)
	if errs != nil {
		for _, err := range errs.(kafka.ProducerErrors) {
			level.Error(c.logger).Log("msg", "Write to kafka failed", "error", err, "batchSize", batchSize)
		}
		return n, errs
	}
	// todo add metrics
	return n, nil
}

// 过滤entry，如果entry line超过最大阀值，日志直接丢弃(只是在kafka端丢弃掉了)
func (c *client) validateEntry(e *api.Entry) bool {
	return len(e.Line) < c.cfg.ProducerMaxMessageSize
}

func (c *client) Name() string {
	// this should be return the url of kafka
	return "kafka"
}

func getFileBaseName(entry *api.Entry) string {
	if value, ok := entry.Labels[FilenameLabel]; ok {
		return path.Base(string(value))
	} else {
		return "UnknownFileName"
	}
}

// DeploymentName get deployment from controllerName
// in common case controllerName is represent the name of replicaset
func DeploymentName(entry *api.Entry) string {
	var controllerName string
	if value, ok := entry.Labels[ControllerNameLabel]; ok {
		controllerName = string(value)
	} else {
		controllerName = "default_controller"
	}
	nSplit := strings.Split(controllerName, "-")
	if len(nSplit) >= 2 {
		return strings.Join(nSplit[:len(nSplit)-1], "-")
	}
	return controllerName
}
