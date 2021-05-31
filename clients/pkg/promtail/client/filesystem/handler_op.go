package filesystem

import (
	"github.com/go-kit/kit/log/level"
	"github.com/sirupsen/logrus"
	"os"
	"path"
	"time"
)

// rotate rotate file
func (h *handler) rotate(newFileName string) error {
	var err error
	oldName := path.Join(h.pathDir, h.fileName)
	if h.fp != nil {
		h.fp.Write([]byte("this is over " + newFileName))
		_ = h.fp.Sync()
		_ = h.fp.Close()
	}
	err = os.Rename(oldName, path.Join(h.pathDir, newFileName))
	if err != nil {
		logrus.Warningf("rename failed  %v --->  %v, err: %v", oldName, newFileName, err)
	}
	h.fp, err = os.OpenFile(oldName, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		logrus.Errorf("open file failed %v", oldName)
	}
	return err
}

// tryReOpen is file handler is close ,then reOpen failed
func (h *handler) tryReOpen() error {
	var err error = nil
	if h.fp != nil {
		_ = h.fp.Close()
		h.fp = nil
	}
	h.fp, err = os.OpenFile(path.Join(h.pathDir, h.fileName), os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		logrus.Error("open failed failed, err: ", err.Error())
	}
	return err
}

// flush sync data to disk
func (h *handler) flush() error {
	// 将数据刷新到磁盘，写入到文件里面
	_, err := h.fp.Write(h.buf.Bytes())
	if err != nil {
		level.Error(h.logger).Log("msg", "flush stream to disk failed"+h.fileName, "err", err.Error())
		return err
	}
	err = h.fp.Sync()
	if err != nil {
		level.Error(h.logger).Log("msg", "sync file to disk failed", "err", err.Error())
		return err
	}
	h.buf.Reset()
	return nil
}

// 根据当前的时间戳判断是否需要更新文件
func (h *handler) checkTruncatePoint() (bool, string) {
	cur := time.Now()                     // 获取当前时间
	year, month, day := h.timeFlag.Date() //获取上次一次更新的timeflag
	future := time.Date(year, month, day, 23, 59, 59, 59, cur.Location())
	// 如果当前时间小于timeflage日期的凌晨，==》还处于当天，则不更新文件句柄
	if cur.UnixNano() <= future.UnixNano() {
		return false, ""
	}
	// update time flag
	// g更新文件句柄
	h.timeFlag = time.Date(year, month, day+1, 0, 0, 0, 0, cur.Location())
	//获取备份文件的名称
	fileBackPostfix := cur.Format("2006-01-02")

	return true, h.fileName + "-" + fileBackPostfix
}



