package filesystem

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/robfig/cron/v3"

	"github.com/grafana/loki/clients/pkg/promtail/fileutil"
)

const (
	DefaultFileHandlerRetry         = 4
	DefaultFileHandlerReSyncPeriod  = 10 * time.Second

)

// 文件操作句柄接口
type Handler interface {
	Receiver() chan string
	Stop()
}

// file handler status begin in initial status

type FileHandler struct {
	// identified a unique handler
	handlerId     string // 文件句柄，todo 是否需要用hash
	directoryTpl  string //  文件目录模版，根据时间日期渲染成具体的目录
	directoryName string
	writeFileName string // 文件名称绝对路径
	logger        log.Logger
	manager       *Manager
	cfg           *FileClientConfig
	isKubernetes  bool
	reSyncPeriod  time.Duration

	retryCount int // 重新尝试次数, 到了一定次数还是异常，则销毁这个handler

	batchBuffer *batch
	watcher     *fsnotify.Watcher

	receiver chan string // entry 接收channel

	handlerContext context.Context
	handlerCancel  context.CancelFunc

	wg sync.WaitGroup

	//fileMissingCh chan struct{}
	rotateCh  chan string // rotateCh
	watcherCh chan struct{}

	// the followed is protected by mu
	mu sync.Mutex
	fp *os.File // handler

}

func (h *FileHandler) Receiver() chan string {
	return h.receiver
}

// newFileHandler is create an file handler instance
//func newFileHandler(parentCancel context.Context, logger log.Logger, handlerId, pathFmt, fileName string, m *Manager, isKubernetes bool, opts ...string) (*FileHandler, error) {
func newFileHandler(parentCancel context.Context, logger log.Logger, cfg *FileClientConfig, manager *Manager, mt *metadata) (*FileHandler, error) {
	var err error
	handler := &FileHandler{
		handlerId:   mt.HandlerId(),
		batchBuffer: newBatch().reSetSize(1 << 6),
		receiver:    make(chan string),
		manager:     manager,
		wg:          sync.WaitGroup{},
		//fileMissingCh: make(chan struct{}),
		rotateCh:      make(chan string),
		watcherCh:     make(chan struct{}),
		mu:            sync.Mutex{},
		writeFileName: mt.fileName,
		directoryTpl:  mt.BasePathFmt(cfg.Path),
		isKubernetes:  mt.isKubernetes,
	}

	if cfg.ReSyncPeriod < 1 {
		handler.reSyncPeriod = DefaultFileHandlerReSyncPeriod
	} else {
		handler.reSyncPeriod = cfg.ReSyncPeriod
	}

	if logger != nil {
		handler.logger = log.With(logger, "Component", "FileHandler")
	} else {
		handler.logger = log.NewNopLogger()
	}

	if handler.retryCount == 0 {
		handler.retryCount = DefaultFileHandlerRetry
	}
	handler.handlerContext, handler.handlerCancel = context.WithCancel(parentCancel)

	if handler.watcher, err = fsnotify.NewWatcher(); err != nil {
		return nil, err
	}

	if err := handler.initial(); err != nil {
		return nil, err
	}

	handler.wg.Add(3)
	// filewatcher handler
	go handler.fileWatcher()
	// main worker
	go handler.run()
	// one cronjob, it will trigger a rotate signal at 0:0 every day
	go handler.cronJob()

	return handler, nil
}

func (h *FileHandler) run() {

	level.Info(h.logger).Log("HandlerId", h.handlerId, "msg", "begin to work")
	ticker := time.NewTicker(h.reSyncPeriod)
	// all handler resource should released in gc method
	defer func() {
		ticker.Stop()
		h.Gc()
		h.wg.Done()
	}()

	for true {
		// if received a stop signal ,then stop
		select {
		case <-h.handlerContext.Done():
			return
		default:
		}

		// if received a rotate signal then stop
		select {
		case newDate := <-h.rotateCh:
			if h.isKubernetes {
				// the logs shouldn't be rotated that is not from container
				if err := h.rotate(newDate, true); err != nil {
					return
				}
			}
		default:
		}

		//// if the file is changed, then reopen it
		select {
		case <-h.watcherCh:
			h.batchBuffer.clean()
			if err := h.reOpen(); err != nil {
				return
			}
		default:
		}

		select {
		// receive a stop signal, then returned
		case <-ticker.C:
			if h.batchBuffer.empty() {
				continue
			}
			if err := h.flush(); err != nil {
				return
			}
		case e := <-h.receiver:
			h.batchBuffer.add(e)
			if !h.batchBuffer.isFull() {
				continue
			}
			if err := h.flush(); err != nil {
				return
			}
		}
	}

}

// rotate directory
// if rotate failed , then exited ,destroy the handler
/*
	1. flush all data to current file
	2. close all file description
	3. create new directory according current date
	4. open new file description and continue
*/
func (h *FileHandler) rotate(newDate string, needFlush bool) error {
	if needFlush {
		if err := h.flush(); err != nil {
			return err
		}
	}
	h.closeFile()

	newDirectory := fmt.Sprintf(h.directoryTpl, newDate)

	if err := os.MkdirAll(newDirectory, os.ModePerm); err != nil {
		return err
	}
	h.directoryName = newDirectory
	if err := h.reOpen(); err != nil {
		return err
	}

	return nil
}

func (h *FileHandler) fileWatcher() {
	var (
		event fsnotify.Event
		ok    bool
	)
	defer func() {
		close(h.watcherCh)
		h.wg.Done()
	}()
	for true {

		select {
		case event, ok = <-h.watcher.Events:
			if !ok {
				continue
			}
		case <-h.handlerContext.Done():
			return
		}
		switch {
		case event.Op&fsnotify.Remove == fsnotify.Remove:
			h.watcherCh <- struct{}{}
		}
	}
}

func (h *FileHandler) initial() error {
	var err error = nil
	var newDirectory string

	if h.isKubernetes {
		newDirectory = fmt.Sprintf(h.directoryTpl, date2String())
	} else {
		newDirectory = h.directoryTpl
	}

	if err := os.MkdirAll(newDirectory, os.ModePerm); err != nil {
		return err
	}
	h.directoryName = newDirectory
	fileName := path.Join(h.directoryName, h.writeFileName)
	h.mu.Lock()
	h.fp, err = fileutil.OpenFile(fileName, h.watcher)
	h.mu.Unlock()
	return err
}

// reOpen if file is removed then reopen the file and continue writing
// reOpen 失败直接注销此handler
// reOpen is goroutine safety, the caller should not add lock before call it
func (h *FileHandler) reOpen() error {
	fileName := path.Join(h.directoryName, h.writeFileName)
	var err error
	_, err = h.fp.Stat()
	if err != nil {
		level.Error(h.logger).Log("state old file failed", fileName, "err", err.Error())
	}
	h.closeFile()
	for _i := 0; _i < h.retryCount; _i++ {
		h.mu.Lock()
		h.fp, err = fileutil.OpenFile(fileName, h.watcher)
		h.mu.Unlock()
		if err == nil{
			_, err = h.fp.Stat()
		}
		if err == nil{
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	level.Error(h.logger).Log("reOpen file failed", h.writeFileName, "err", err.Error())
	return err
}

func (h *FileHandler) closeFile() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := h.watcher.Remove(path.Join(h.directoryName, h.writeFileName)); err != nil {
		level.Error(h.logger).Log("remove filewatcher", path.Join(h.directoryName, h.writeFileName), "err", err.Error())
	}
	if h.fp != nil {
		if err := h.fp.Close(); err != nil {
			level.Error(h.logger).Log("close file failed,", path.Join(h.directoryName, h.writeFileName), "err", err.Error())
		}
		h.fp = nil
	}
}

// check current timestamp is current, if not then rotate file
func (h *FileHandler) cronJob() {
	defer func() {
		close(h.rotateCh)
		h.wg.Done()
	}()
	parser := cron.NewParser(
		cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)
	c := cron.New(cron.WithParser(parser))
	c.AddFunc("1 1 0 * * *", func() {
		level.Info(h.logger).Log("msg", "rotate handler", "time", date2String())
		h.rotateCh <- date2String()
	})
	c.Start()
	defer func() {
		c.Stop()
	}()
	for true {
		select {
		case <-h.handlerContext.Done():
			return
		}
	}
}

// clean all resource for ready stop
func (h *FileHandler) Stop() {
	h.handlerCancel()
	h.wg.Wait()
}

// flush buffer to file ,if failed and try matched the max try number, then kill this handler
// if flush failed, then reOpen the file and continue, in this case the batchBuffer will be dropped
func (h *FileHandler) flush() error {
	defer func() {
		h.batchBuffer.clean()
	}()
	//defer h.batchBuffer.clean()
	if h.batchBuffer.size() == 0 {
		return nil
	}
	if h.fp == nil {
		return fmt.Errorf("file description is nil")
	}
	h.mu.Lock()
	_, err := h.fp.Write(h.batchBuffer.encode())
	h.mu.Unlock()
	if err == nil {
		if err = h.fp.Sync(); err == nil {
			return nil
		}
	}

	level.Error(h.logger).Log("Flush Buffer Failed", h.handlerId, "err", err.Error())
	// 第一次写入失败，后面大概率基本上也是失败, 此时应该去重新reOpenfile
	return h.reOpen()
}

// Gc is used to clean all resources
func (h *FileHandler) Gc() {
	if err := h.flush(); err != nil {
		level.Error(h.logger).Log("GC Flush buffer failed", h.handlerId, "err", err.Error())
	}

	h.closeFile()
	if h.watcher != nil {
		h.watcher.Close()
	}

	if h.manager != nil {
		if err := h.manager.unRegister(h.handlerId); err != nil {
			level.Error(h.logger).Log("GC UnRegister handler", h.handlerId, "err", err.Error())
		}
	}
}


// dateToday return an date string according
func date2String(opts ...string) string {
	if len(opts) > 0 {
		return opts[0]
	}
	local,_ := time.LoadLocation("Asia/Shanghai")
	year, month, day := time.Now().In(local).Date()
	return fmt.Sprintf("%d-%d-%d", year, month, day)
}
