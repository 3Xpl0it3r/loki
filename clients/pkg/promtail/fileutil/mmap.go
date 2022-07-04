package fileutil

import (
	"os"
	"github.com/pkg/errors"
)

type MMapFile struct {
	f *os.File
	b []byte
}


func OpenMMapFile(path string)(*MMapFile, error){
	return openMMapFileWithSize(path, 0)
}

func openMMapFileWithSize(path string, size int)(mf *MMapFile,retErr error){
	f,err := os.Open(path)
	if err != nil{
		return nil, err
	}
	defer func() {
		if retErr != nil{
			f.Close()
		}
	}()
	if size <= 0 {
		info ,err := f.Stat()
		if err != nil{
			return nil, errors.Wrapf(err, "file stat")
		}
		size = int(info.Size())
	}
	b,err := mmap(f, size)
	if err != nil{
		return nil, errors.Wrapf(err, "mmap")
	}
	return &MMapFile{f: f, b: b}, nil
}


func(f *MMapFile)Close()error{
	err0 := munmap(f.b)
	err1 := f.f.Close()
	if err0 != nil{
		return err0
	}
	return err1
}

func(f *MMapFile)File()*os.File{
	return f.f
}

func(f *MMapFile)Bytes()[]byte{
	return f.b
}