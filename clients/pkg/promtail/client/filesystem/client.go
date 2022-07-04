package filesystem

import (
	"context"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/loki/clients/pkg/promtail/api"
	clientsmetrics "github.com/grafana/loki/clients/pkg/promtail/client/metrics"
	"github.com/prometheus/common/model"
	"os"
	"sync"
)

// const label is used for mark directory and filename

var (
	once sync.Once

	NamespaceLabel      model.LabelName = "namespace"
	ControllerNameLabel model.LabelName = "controller_name"
	InstanceLabel       model.LabelName = "instance"
	FileNameLabel       model.LabelName = "filename"

	defaultNamespace      = "default"
	defaultControllerName = "default_controller"
	defaultInstanceName   = "default_instance"
)

func init(){
	once.Do(func() {
		defaultInstanceName, _ = os.Hostname()
	})
}


// client 描述客户端所需要的信息
type client struct {
	cfg     FileClientConfig
	metrics *clientsmetrics.Metrics
	streamLagLabels []string
	logger  log.Logger
	entries chan api.Entry // 接收来自client的entry

	ctx    context.Context
	cancel context.CancelFunc

	wg   sync.WaitGroup
	once sync.Once


	manager *Manager
}

// 文件处理逻辑，handlerId,
//
func NewFileSystemClient(metrics *clientsmetrics.Metrics, cfg FileClientConfig, streamLagLabels []string, logger log.Logger) (*client, error) {
	client := &client{
		cfg:     cfg,
		logger:  log.With(logger, "client_type", "filesystem"),
		entries: make(chan api.Entry),
		streamLagLabels: streamLagLabels,
		wg:      sync.WaitGroup{},
		once:    sync.Once{},
		manager: newHandlerManager(),
        metrics: metrics,
	}
	client.ctx, client.cancel = context.WithCancel(context.Background())
	// run client main loop
	client.wg.Add(1)
	go client.run()
	return client, nil
}

// run function dispatch entry to all relative file handler
func (c *client) run() {
	defer c.wg.Done()
	level.Info(c.logger).Log("msg", "filesystem client start running...")
	for e := range c.entries {
		c.send(&e)
	}
}

func (c *client) send(e *api.Entry) {
	defer func() {
		if r := recover(); r != nil {
			level.Error(c.logger).Log("msg", "send accrue an panic error", "err", r)
		}
	}()
	md := Get()
	err := parseEntry(e, md)
	if err != nil {
		level.Error(c.logger).Log("msg", "get metadata failed", "err", err.Error())
		return
	}
	handler := c.manager.Get(md.HandlerId())
	if handler == nil {
		handler, err = newFileHandler(c.ctx, c.logger, &c.cfg, c.manager, md)
		if err != nil {
			level.Error(c.logger).Log("msg", "generate new handler failed", "err", err.Error())
			return
		}
		if err = c.manager.Register(md.HandlerId(), handler); err != nil {
			// if register failed, then destroy the handler, return
			handler.Stop()
			level.Error(c.logger).Log("msg", "register failed", "err", err.Error())
			goto  GC
		}
	}
	handler.Receiver() <- e.Line
GC:
	Put(md)
}

func (c *client) Chan() chan<- api.Entry {
	return c.entries
}

func (c *client) Stop() {
	c.once.Do(func() {
		close(c.entries)
		c.cancel()
	})
	if err := c.manager.AwaitComplete();err != nil{
		// todo log
	}
	c.wg.Wait()
}

func (c *client) StopNow() {
	c.Stop()
}


func(c *client)Name()string{
	// this should return the path
	return "filesystem"
}
