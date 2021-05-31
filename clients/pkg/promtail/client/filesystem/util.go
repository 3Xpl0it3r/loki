package filesystem

import (
	"errors"
	"os"
)

// 检测file handler is validate
func validateFileHandler(fp *os.File)error{
	if fp == nil{
		return errors.New("file handler is empty")
	}
	if _, err := fp.Stat();err != nil{
		return err
	}
	return nil
}

func createDirectoryIfNotExisted(dir string) error {
	if _, err := os.Stat(dir); err != nil {
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return err
		}
	}
	return nil
}