package container

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/mount"
	docker "github.com/docker/docker/client"
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

type dockerClient struct {
	client *docker.Client
}

var _ Client = new(dockerClient)

// newDocker return docker
func newDocker() (*dockerClient, error) {

	client, err := docker.NewClientWithOpts(docker.FromEnv, docker.WithAPIVersionNegotiation())
	return &dockerClient{client}, err
}

// Stop [#TODO](should add some comments)
func (d *dockerClient) Stop() {
    if d.client!= nil {
        d.client.Close()
    }
}

// Ping check dockerClient is reachedAble
func (d *dockerClient) Ping() error {

	_, err := d.client.Ping(context.TODO())
	if err != nil {
		return err
	} else {
		return nil
	}
}

// LogStoreDir [#TODO](should add some comments)
func (d *dockerClient) LogStoreDir(containerId string) (string, string, error) {
	inspect, err := d.containerInfo(containerId)
	if err != nil {
		return "", "", fmt.Errorf("Get Container Information failed: %v", err)
	}

	external := mountPoint(inspect)
	internal := graphDir(inspect)

	if external == "" && internal == "" {
		return "", "", fmt.Errorf("Graph and Volumn cannot both empty ")
	}

	return external, internal, nil
}

func (d *dockerClient) containerInfo(containerId string) (*types.ContainerJSON, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inspect, err := d.client.ContainerInspect(ctx, containerId)
	if err != nil {
		return nil, err
	}
	return &inspect, err
}

// same as MountsVolumes
func mountPoint(inspect *types.ContainerJSON) string {
	for _, mountPoint := range inspect.Mounts {
		if mountPoint.Type != mount.TypeBind {
			continue
		}
		if mountPoint.Source == "/data/logs" && mountPoint.Destination == "/root/logs" {
			return mountPoint.Source
		}
	}
	return ""
}

// same as GraphDriverUpperDir
func graphDir(inspect *types.ContainerJSON) string {
	for key, value := range inspect.GraphDriver.Data {
		if key == string(GraphUpperDirDir) {
			return value
		}
	}
	return ""
}
