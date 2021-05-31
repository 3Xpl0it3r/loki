package filesystem

import (
	"context"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/grafana/loki/clients/pkg/promtail/api"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"path"

	"sync"
)

// const label is used for mark directory and filename
const (
	// client file is only support k8s
	NamespaceLabel      model.LabelName = "namespace"
	ControllerNameLabel model.LabelName = "controller_name"
	InstanceLabel       model.LabelName = "instance"

	defaultNamespace      = "default_namespace"
	defaultControllerName = "default_controller"
	defaultInstanceName   = "default_instance"
)


// metadata 写文件所需要的元数据，group/service/app 拼接目录
type metadata struct {
	namespace      string
	controllerName string
	instance       string
	fileName       string
	originFilename string
}

func (m metadata) Identifier() string {
	return m.instance + "-" + m.originFilename
}
func (m metadata) FileName() string {
	return m.fileName
}
func (m metadata) RelativePath() string {
	return m.namespace + "/" + m.controllerName + "/" + m.instance
}

// 文件操作句柄接口
type Handler interface {
	// 推出关闭，清理资源
	close()
	//  开始运行handler，每个handler都打开一个文件，写文件
	run(ctx context.Context)
	// 从client接收资源
	Chan() chan<- api.Entry
}

// client 描述客户端所需要的信息
type client struct {
	fpHandlers map[string]Handler
	cfg        FileClientConfig
	// 增加prometheus的监控
	reg prometheus.Registerer
	// 日志记录模块
	logger log.Logger

	stopFn  context.CancelFunc
	entries chan api.Entry // 接收来自client的entry
	once    sync.Once
}

func NewFileSystemClient(reg prometheus.Registerer, cfg FileClientConfig, logger log.Logger) (*client, error) {
	client := &client{
		cfg:        cfg,
		reg:        reg,
		logger:     log.With(logger, "client_type", "filesystem"),
		entries:    make(chan api.Entry),
		once:       sync.Once{},
		fpHandlers: make(map[string]Handler),
	}
	ctx, cancel := context.WithCancel(context.Background())
	client.stopFn = cancel
	go client.run(ctx, reg, cfg, logger)
	return client, nil
}

// run function dispatch entry to all relative file handler
func (c *client) run(ctx context.Context, reg prometheus.Registerer, cfg FileClientConfig, logger log.Logger) {
	for {
		select {
		case e, ok := <-c.entries:
			if !ok {
				continue
			}
			metadata, err := generateMetadata(&e)
			if err != nil {
				level.Error(c.logger).Log("msg", "get metadata failed", "err", err.Error())
				continue
			}

			// 检测对应的handler是否存在，不存在则创建一个新的handler
			handler, ok := c.fpHandlers[metadata.Identifier()]
			if !ok {
				handler, err = newHandler(reg, cfg, logger, metadata)
				if err != nil {
					level.Error(c.logger).Log("msg", "generate newhandler failed", "err", err.Error())
					continue
				}
				c.fpHandlers[metadata.Identifier()] = handler
				level.Debug(c.logger).Log("msg", "register handler successfully", "id", metadata.instance)
			}
			if handler == nil {
				continue
			}
			handler.Chan() <- e
		case <-ctx.Done():
			break
		}
	}
}

// 接收数据
func (c *client) Chan() chan<- api.Entry {
	return c.entries
}

// 暂停
func (c *client) Stop() {
	c.stopFn()
	c.once.Do(func() {
		close(c.entries)
		for _, handler := range c.fpHandlers {
			handler.close()
		}
	})
}

// 立即停止
func (c *client) StopNow() {
	c.Stop()
}

//根据label生成元数据
func generateMetadata(entry *api.Entry) (metadata, error) {
	var (
		namespace      string
		instance       string
		controllerName string
		fileName       string
		originFileName string
	)
	if value, ok := entry.Labels[NamespaceLabel]; ok {
		namespace = string(value)
	} else {
		namespace = defaultNamespace
	}
	if value, ok := entry.Labels[ControllerNameLabel]; ok {
		controllerName = string(value)
	} else {
		controllerName = defaultControllerName
	}
	if value, ok := entry.Labels[InstanceLabel]; ok {
		instance = string(value)
	} else {
		instance = defaultInstanceName
	}
	if value, ok := entry.Labels[model.LabelName("filename")]; ok {
		originFileName = string(value)
		fileName = getFileBaseName(string(value))
	} else {
		return metadata{}, errors.New("path label must be existed")
	}
	return metadata{
		namespace:      namespace,
		controllerName: controllerName,
		instance:       instance,
		fileName:       fileName,
		originFilename: originFileName,
	}, nil
}

func getFileBaseName(fileName string) string {
	return path.Base(fileName)
}
