package filesystem

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/grafana/loki/clients/pkg/promtail/api"
	"github.com/prometheus/client_golang/prometheus"

	"sync"
)

const (
	FileCount = 10
)

// handler is used for write file
type handler struct {
	cfg    FileClientConfig // 客户端配置文件
	metadata metadata       // 创建文件所需要的元数据
	logger log.Logger       //记录日志
	entries  chan api.Entry // 接收来自client的entry
	// timeFlag is used for
	timeFlag time.Time // today

	buf      *bytes.Buffer  // 缓冲区


	// path
	fp       *os.File
	// root directory
	pathDir  string
	// fileName
	fileName string
	// retry
	retry int32
	// fake test
	fileCount int32

	once   sync.Once
	cancel context.CancelFunc
	lock     sync.RWMutex
}



// newHandler return instance of handler
func newHandler(reg prometheus.Registerer, cfg FileClientConfig, logger log.Logger, meta metadata) (Handler, error) {
	h := &handler{
		once:      sync.Once{},
		buf:       bytes.NewBuffer([]byte("")),
		metadata:  meta,
		logger:    log.With(logger, "handler_id", meta.namespace+"-"+meta.controllerName+"-"+meta.instance),
		entries:   make(chan api.Entry),
		cfg:       cfg,
		timeFlag:  time.Now(),
		fileCount: 0,
	}

	h.pathDir = path.Join(cfg.Path, meta.RelativePath())
	h.fileName = meta.FileName()

	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go h.run(ctx)
	return h, nil
}


func (h *handler) run(ctx context.Context) {
	level.Debug(h.logger).Log("msg", "begin run handler")
	// generate some necessary directory
	if err := createDirectoryIfNotExisted(h.pathDir);err != nil{
		panic(fmt.Errorf("create directory failed,,%v, err:%v", h.pathDir, err.Error()))
	}
	// valid file handler is valid or not ,if invalid ,then open
	if err := validateFileHandler(h.fp); err != nil {
		if err := h.tryReOpen(); err != nil {
			panic(fmt.Errorf("reOpenFile Failed, Dir:%v  File:%v Err:%v", h.pathDir, h.fileName, err.Error()))
		}
	}

	minWaitCheckFrequency := 10 * time.Millisecond
	maxWaitCheckFrequency := h.cfg.BatchWait / 10
	if maxWaitCheckFrequency < minWaitCheckFrequency {
		maxWaitCheckFrequency = minWaitCheckFrequency
	}
	maxWaitCheck := time.NewTicker(maxWaitCheckFrequency)
	defer func() {
		maxWaitCheck.Stop()
		// 推出时候将剩余的数据一次性的同步到磁盘
		_ = h.flush()
	}()

	for {
		select {
		case e, ok := <-h.entries:
			// 获取到条目
			if !ok {
				level.Debug(h.logger).Log("msg", "get entries failed")
				return
			}
			if h.buf.Len() > h.cfg.BatchSize {
				_ = h.flush()
				if ok,backFilename := h.checkTruncatePoint(); ok {
					_ = h.rotate(backFilename)
				}
				continue
			}
			h.buf.Write([]byte(e.Line + "\n"))

		case <-maxWaitCheck.C:
			_ = h.flush()
			if ok,backFilename := h.checkTruncatePoint(); ok {
				_ = h.rotate(backFilename)
			}
		case <-ctx.Done():
			break
		}
	}
}


func (h *handler) Chan() chan<- api.Entry {
	return h.entries
}

// close shutdown handler
func (h *handler) close() {
	close(h.entries)
	h.cancel()
	if h.fp != nil {
		err := h.fp.Close()
		if err != nil {
			level.Error(h.logger).Log("msg", "close handler , close file description failed", "err", err)
		}
	}
}











