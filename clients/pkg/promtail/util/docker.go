package util

import (
	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/pkg/errors"
	"sync"
)

var (
	once   sync.Once
	docker *DockerClient
)

type DockerGraphDirType string
type MountType string

const (
	// lowerdir is diff ignore
	GraphLowerDir DockerGraphDirType = "LowerDir"
	// upper dir diff
	GraphUpperDirDir DockerGraphDirType = "UpperDir"
	// workdir work
	GraphWorkDirDir DockerGraphDirType = "WorkDir"

	MountTypeBind   = "bind"
	MountTypeVolume = "volume"
)

type DockerClient struct {
	client *client.Client
}

// NewDockerClient return docker
func NewDockerClient() *DockerClient {

	if docker == nil {
		once.Do(func() {
			client, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
			if err != nil {
				panic("new docker client failed")
			}
			docker = &DockerClient{client: client}
		})
	}
	return docker
}

func (d *DockerClient) Volumes(containerId string) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inspect, err := d.client.ContainerInspect(ctx, containerId)
	if err != nil {
		return "", err
	}
	for key, _ := range inspect.Config.Volumes {
		if key != "" {
			return key, nil
		}
	}
	return "", errors.New("not volume in this container " + containerId)
}

// get docker console log path, this is need be collected by promtail
func (d *DockerClient) ConsoleLogPath(containerId string) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inspect, err := d.client.ContainerInspect(ctx, containerId)
	if err != nil {
		return "", err
	}
	return inspect.LogPath, nil
}

func (d *DockerClient) DockerInspect(containerId string) (*types.ContainerJSON, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inspect, err := d.client.ContainerInspect(ctx, containerId)
	if err != nil {
		return nil, err
	}
	return &inspect, err
}

// same as MountsVolumes
func GetDockerVolumePath(inspect *types.ContainerJSON) (string, error) {
	if inspect == nil {
		return "", errors.New("inspect information is nil")
	}
	for _, mountPoint := range inspect.Mounts {
		if mountPoint.Type == mount.TypeVolume {
			return mountPoint.Source, nil
		}
	}
	return "", errors.New("Not Found Mount Volume")
}

// same as GraphDriverUpperDir
func GetDockerDataPath(inspect *types.ContainerJSON) (string, error) {
	if inspect == nil {
		return "", errors.New("inspect information is nil")
	}
	for key, value := range inspect.GraphDriver.Data {
		if key == string(GraphUpperDirDir) {
			return value, nil
		}
	}
	return "", errors.New("not found diff volume in docker")
}

// MountsVolumes return the Mounts that returned by docker inspect information
func (d *DockerClient) MountsVolumes(containerId string) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inspect, err := d.client.ContainerInspect(ctx, containerId)
	if err != nil {
		return "", err
	}
	for _, mountPoint := range inspect.Mounts {
		if mountPoint.Type == mount.TypeVolume {
			return mountPoint.Source, nil
		}
	}
	return "", err
}

// GraphDriverUpperDir return UpperDir data that returned by inspect information
func (d *DockerClient) GraphDriverUpperDir(containerId string) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inspect, err := d.client.ContainerInspect(ctx, containerId)
	if err != nil {
		return "", err
	}
	for key, value := range inspect.GraphDriver.Data {
		if key == string(GraphUpperDirDir) {
			return value, nil
		}
	}

	return "", errors.New("not found diff volume in docker")
}

// Ping check dockerClient is reachedAble
func (d *DockerClient) Ping() error {

	_, err := d.client.Ping(context.TODO())
	if err != nil {
		return err
	} else {
		return nil
	}
}
