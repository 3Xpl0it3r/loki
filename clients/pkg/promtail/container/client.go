package container

import (
	"fmt"
	"os"
)

const (
	DefaultDockerSocket     = "/var/run/docker.sock"
	DefaultContainerdSocket = "/run/containerd/containerd.sock"
)

type Client interface {
    Ping()error
    LogStoreDir(containerId string)(string, string, error)
    Stop()
}

func NewClient() (Client, error) {
	if _, err := os.Stat(DefaultDockerSocket); err == nil {
        return newDocker()
	}

	if _, err := os.Stat(DefaultContainerdSocket); err == nil {
		return newContainerd(DefaultContainerdSocket)
	}

	return nil, fmt.Errorf("Could not create Cri Client")
}
