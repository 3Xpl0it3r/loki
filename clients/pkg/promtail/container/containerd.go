package container

import (
	"context"
	"encoding/json"
	"fmt"

	containerd "github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
)

const (
	// TODO Hard code here tempoary, cann't get container workdir throug container client for now
	DefaultContainerStateDir = "/var/run/containerd"
	DefaultMergedDir = DefaultContainerStateDir + "/io.containerd.runtime.v2.task/k8s.io"
)

// Spec represent spec
type containerSpec struct {
	OciVersion string `json:"ociversion"`
	Mounts     []struct {
		Destination string `json:"destination"`
		Type        string `json:"type"`
		Source      string `json:"source"`
	} `json:"mounts"`
}

type containerdClient struct {
	client *containerd.Client
}

// Stop implements Client.
func (c *containerdClient) Stop() {
	if c.client != nil {
		c.client.Close()
	}

}

var _ Client = new(containerdClient)

func newContainerd(sk string) (*containerdClient, error) {
	client, err := containerd.New(sk)
	if err != nil {
		return nil, err
	}
	return &containerdClient{client}, nil
}

// Ping [#TODO](should add some comments)
func (c *containerdClient) Ping() error {
    return nil
}

// LogStoreDir implements Client.
func (c *containerdClient) LogStoreDir(containerId string) (string, string, error) {
	spec, err := c.containerInfo(containerId)
	if err != nil {
		return "", "", fmt.Errorf("LogStoreDir failed: %v", err)
	}
	for _, mountPoint := range spec.Mounts {

		if mountPoint.Type != "bind" {
			continue
		}
		if mountPoint.Source == "/data/logs" && mountPoint.Destination == "/root/logs" {
			return mountPoint.Source, "", nil
		}
	}
	return "", DefaultMergedDir + "/" + containerId + "/rootfs/", nil
}

// containerInfo [#TODO](should add some comments)
func (c *containerdClient) containerInfo(containerId string) (*containerSpec, error) {
	container, err := c.client.LoadContainer(namespaces.WithNamespace(context.Background(), "k8s.io"), containerId)
	if err != nil {
		return nil, err
	}
	info, err := container.Info(namespaces.WithNamespace(context.Background(), "k8s.io"), containerd.WithoutRefreshedMetadata)
	if info.Spec == nil || info.Spec.GetValue() == nil {
		return nil, fmt.Errorf("Get containerInfo failed: %v", err)
	}

	var containerSpec containerSpec
	err = json.Unmarshal(info.Spec.GetValue(), &containerSpec)
	if err != nil {
		return nil, fmt.Errorf("Unmarshal spec failed: %v", err)
	}
	return &containerSpec, nil
}
