package filesystem

import (
	"bytes"
	"errors"
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

// handler is used for write file
type handler struct {
	buf *bytes.Buffer			// 缓冲区
	metadata metadata			// 创建文件所需要的元数据
	entries chan api.Entry	// 接收来自client的entry
	fp *os.File

	logger log.Logger 			//记录日志
	cfg FileClientConfig		// 客户端配置文件
	once sync.Once
	wg   sync.WaitGroup

	timeFlag time.Time		// today

	lock sync.RWMutex
	// path
	pathDir string
	fileName string

}



func newHandler(reg prometheus.Registerer, cfg FileClientConfig, logger log.Logger, meta metadata)(Handler,error){
	h := &handler{
		once:       sync.Once{},
		wg:         sync.WaitGroup{},
		buf :bytes.NewBuffer([]byte("")),
		metadata: meta,
		logger: log.With(logger, "handler_id", meta.namespace+"-"+meta.controllerName+"-"+meta.instance),
		entries: make(chan api.Entry),
		cfg: cfg,
		timeFlag: time.Now(),
	}
	absPath := path.Join(cfg.Path, meta.RelativePath())
	h.pathDir = absPath
	h.fileName = meta.FileName()
	err := createDirectoryIfNotExisted(absPath)
	if err != nil{
		level.Error(h.logger).Log("create directory" + "/" + meta.RelativePath(), "msg", err.Error())
		return nil, err
	}
	// 检测文件是否存在，不存在则创建新的
	filename := path.Join(absPath, meta.FileName())
	fp ,err := generateFileHandler(filename)
	if err != nil{
		h.logger.Log("cretate file pointee")
		return nil, errors.New(fmt.Sprintf("generate file failed, err: %s",err.Error()))
	}
	h.fp = fp
	h.wg.Add(1)
	go h.run()
	return h, nil
}

func (h *handler)run(){
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
		h.flush()
		h.wg.Done()
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
				h.flush()
				continue
			}
			h.buf.Write([]byte(e.Line + "\n"))

		case <-maxWaitCheck.C:
			// Send all batches whose max wait time has been reached
			// need rollback log
			h.flush()
		}
	}
}

func (h *handler)updateFileConcur(){

}

func (h *handler)flush(){
	// 刷新数据之前先检测是否需要备份文件
	if err := h.flushFilePointer(); err != nil{
		level.Error(h.logger).Log("msg", "backup and update file pointer failed", "err", err.Error())
		return
	}
	// 将数据刷新到磁盘，写入到文件里面
	_,err := h.fp.Write(h.buf.Bytes())
	if err != nil{
		level.Error(h.logger).Log("msg", "flush stream to disk failed", "err", err.Error())
		return
	}
	err = h.fp.Sync()
	if err != nil{
		level.Error(h.logger).Log("msg", "sync file to disk failed", "err", err.Error())
		return
	}
	h.buf.Reset()
}

func (h *handler) Chan() chan<- api.Entry {
	return h.entries
}


// close shutdown handler
func (h *handler) close() {
	close(h.entries)
	if h.fp != nil{
		err := h.fp.Close()
		if err != nil{
			level.Error(h.logger).Log("msg", "close handler , close file description failed", "err", err)
		}
	}
	h.wg.Wait()
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
			fp,err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
			if err != nil{
			} else {

			}
		}
	}
	return fp, nil
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
		return err
	}
	h.fp = fp
	return nil
}


