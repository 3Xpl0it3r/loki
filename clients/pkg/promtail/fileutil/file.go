package fileutil

import (
	"github.com/fsnotify/fsnotify"
	"os"
)

func OpenFile(name string, watcher *fsnotify.Watcher) (file *os.File, err error) {
	filename := name

	fi, err := os.Lstat(name)
	if err == nil {
		if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
			filename, err = os.Readlink(name)
			if err != nil {
				return nil, err
			}
		}
	}

	fp, err := os.OpenFile(filename, os.O_APPEND| os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil{
		return nil, err
	}
	if watcher != nil{
		watcher.Add(filename)
	}
	return fp, nil
}


