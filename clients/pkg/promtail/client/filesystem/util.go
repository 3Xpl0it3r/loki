package filesystem

import (
	"crypto/md5"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
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


// DeploymentName get deployment from controllerName
// in common case controllerName is represent the name of replicaset
func DeploymentName(controllerName string)string{
	nSplit := strings.Split(controllerName, "-")
	if len(nSplit) >=2 {
		return strings.Join(nSplit[:len(nSplit)-1], "-")
	}
	return controllerName
}


// dateToday return an date string according
func dateToday()string{
	year, month, day := time.Now().Date()
	return fmt.Sprintf("%d-%d-%d", year, month, day)
}


func hashForIdentifier(in string)string{
	hash := md5.New()
	_,err := hash.Write([]byte(in))
	if err != nil{
		return in
	}
	result := hash.Sum([]byte(fmt.Sprintf("%d", time.Now().Nanosecond())))
	return fmt.Sprintf("%x", result)
}