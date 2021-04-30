package util

import (
	"context"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"sync"
)

var (
	once sync.Once
	docker *DockerClient
)


type DockerGraphDirType string

const (
	// lowerdir is diff ignore
	GraphLowerDir DockerGraphDirType = "LowerDir"
	// upper dir diff
	GraphUpperDirDir DockerGraphDirType = "UpperDir"
	// workdir work
	GraphWorkDirDir DockerGraphDirType = "WorkDir"
)

type DockerClient struct {
	client *client.Client
}

// NewDockerClient return docker
func NewDockerClient()*DockerClient{
	if docker == nil{
		once.Do(func() {
			client,err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
			if err != nil{
				logrus.Panic("new docker client failed")
			}
			docker = &DockerClient{client: client}
		})
	}
	return docker
}


func(d *DockerClient)Volumes(containerId string)(string,error){
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inspect,err := d.client.ContainerInspect(ctx, containerId)
	if err != nil{
		return "", err
	}
	for key,_ := range inspect.Config.Volumes{
		if key != ""{
			return key, nil
		}
	}
	return "", errors.New("not volume in this container " + containerId)
}



// GraphDiff show diff
func(d *DockerClient)GraphDriverUpperDir(containerId string)(string,error){
	ctx,cancel := context.WithCancel(context.Background())
	defer cancel()
	inspect,err := d.client.ContainerInspect(ctx, containerId)
	if err != nil{
		return "", err
	}
	for key, value := range inspect.GraphDriver.Data{
		if key == string(GraphUpperDirDir){
			return value, nil
		}
	}

	return "", errors.New("not found diff volume in docker")
}


// Ping check dockerClient is reachedAble
func(d *DockerClient)Ping()error{

	_,err := d.client.Ping(context.TODO())
	if err != nil{
		return err
	} else {
		return nil
	}
}