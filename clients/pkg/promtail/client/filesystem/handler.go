package filesystem

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sync/atomic"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/grafana/loki/clients/pkg/promtail/api"
	"github.com/prometheus/client_golang/prometheus"

	"sync"
)

// handler is used for write file
type handler struct {
	buf *bytes.Buffer			// 缓冲区
	metadata metadata			// 创建文件所需要的元数据
	entries chan api.Entry	// 接收来自client的entry
	fp *os.File

	logger log.Logger 			//记录日志
	cfg FileClientConfig		// 客户端配置文件
	once sync.Once
	cancel context.CancelFunc

	timeFlag time.Time		// today
	lock sync.RWMutex
	// path
	pathDir string
	fileName string


	retry int32

}

func newHandler(reg prometheus.Registerer, cfg FileClientConfig, logger log.Logger, meta metadata)(Handler,error){
	h := &handler{
		once:       sync.Once{},
		buf :bytes.NewBuffer([]byte("")),
		metadata: meta,
		logger: log.With(logger, "handler_id", meta.namespace+"-"+meta.controllerName+"-"+meta.instance),
		entries: make(chan api.Entry),
		cfg: cfg,
		timeFlag: time.Now(),
	}

	h.pathDir = path.Join(cfg.Path, meta.RelativePath())
	h.fileName = meta.FileName()
	err := createDirectoryIfNotExisted(h.pathDir)
	if err != nil{
		level.Error(h.logger).Log("create directory" + "/" + meta.RelativePath(), "msg", err.Error())
		return nil, err
	}
	// 检测文件是否存在，不存在则创建新的
	//filename := path.Join(absPath, meta.FileName())
	if err := h.reOpenFile(); err != nil{
		level.Error(h.logger).Log("msg","generate file " + path.Join(h.pathDir, h.fileName),"err", err.Error())
		h.logger.Log("cretate file pointee")
		return nil, errors.New(fmt.Sprintf("generate file failed, err: %s",err.Error()))
	}
	ctx,cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go h.run(ctx)
	return h, nil
}

func (h *handler)run(ctx context.Context){
	level.Debug(h.logger).Log("msg", "begin run handler")
	minWaitCheckFrequency := 10 * time.Millisecond
	maxWaitCheckFrequency := h.cfg.BatchWait/ 10
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
			if h.buf.Len() > h.cfg.BatchSize{
				if err := h.flush(); err != nil{
					if err := h.reOpenFile(); err != nil && h.retry < 10{
						atomic.AddInt32(&h.retry, 1)
					} else {
						goto EXIST
					}
				}
				continue
			}
			h.buf.Write([]byte(e.Line + "\n"))

		case <-maxWaitCheck.C:
			// Send all batches whose max wait time has been reached
			// need rollback log
			_ = h.flush()
		case <- ctx.Done():
			break
		}
	}
	EXIST:
	h.logger.Log("msg","handler existed", "file", h.fileName)

}


func (h *handler)flush() error{
	// 刷新数据之前先检测是否需要备份文件
	if err := h.flushFilePointer(); err != nil{
		level.Error(h.logger).Log("msg", "backup and update file pointer failed", "err", err.Error())
		return err
	}
	// 将数据刷新到磁盘，写入到文件里面

	_,err := h.fp.Write(h.buf.Bytes())
	if err != nil{
		level.Error(h.logger).Log("msg", "flush stream to disk failed" + h.fileName, "err", err.Error())
		return err
	}
	err = h.fp.Sync()
	if err != nil{
		level.Error(h.logger).Log("msg", "sync file to disk failed", "err", err.Error())
		return err
	}
	h.buf.Reset()
	return nil
}

func (h *handler) Chan() chan<- api.Entry {
	return h.entries
}


// close shutdown handler
func (h *handler) close() {
	close(h.entries)
	h.cancel()
	if h.fp != nil{
		err := h.fp.Close()
		if err != nil{
			level.Error(h.logger).Log("msg", "close handler , close file description failed", "err", err)
		}
	}
}


func createDirectoryIfNotExisted(dir string)error{
	if _, err := os.Stat(dir); err != nil {
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return err
		}
	}
	return nil
}



func generateFileHandler(filename string)(*os.File, error){
	// 检测文件是否存在，不存在则创建新的文件句柄
	var fp *os.File
	_,err := os.Stat(filename)
	if err != nil{
		if os.IsNotExist(err){
			fp,err = os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil{
				return nil, err
			}
		}
		return nil, err
	}
	return fp, nil
}



func(h *handler)reOpenFile()(err error){
	if h.fp != nil{
		if err =h.fp.Close(); err != nil{
			return err
		}
	}
	fp,err := os.OpenFile(path.Join(h.pathDir, h.fileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil{
		return err
	}
	h.fp = fp
	return nil
}

// 根据当前的时间戳判断是否需要更新文件
func (h *handler)flushFilePointer()error{
	cur := time.Now()	// 获取当前时间
	year,month,day := h.timeFlag.Date() 	//获取上次一次更新的timeflag
	future := time.Date(year, month, day, 23,59,59, 59,cur.Location())
	// 如果当前时间小于timeflage日期的凌晨，==》还处于当天，则不更新文件句柄
	if cur.UnixNano() <= future.UnixNano(){
		return nil
	}
	// update time flag
	// g更新文件句柄
	h.timeFlag = time.Date(year, month, day+1, 0, 0, 0,0, cur.Location())
	//获取备份文件的名称
	fileBackPostfix := cur.Format("2006-01-02")
	h.lock.Lock()
	defer h.lock.Unlock()
	h.fp.Close()	// 关闭关闭文件句柄
	originName := path.Join(h.pathDir, h.fileName)
	newFileName := path.Join(h.pathDir, h.fileName + "-" +fileBackPostfix)
	e := os.Rename(originName, newFileName)
	if e != nil{
		return e
	}
	// 重新生成句柄
	fp,err := generateFileHandler(path.Join(h.pathDir, h.fileName))
	if err != nil{
		if fp != nil{fp.Close()}
		return err
	}
	h.fp = fp
	return nil
}


