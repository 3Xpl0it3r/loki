package filesystem

import (
	"github.com/grafana/loki/clients/pkg/promtail/api"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

const (
	FakeNamespace     = "fakeNamespace"
	FakeController    = "fakeController"
	FakeInstance      = "fakeInstance"
	FakeFileName      = "fakeDir1/fakeDir2/fakeFile"
	FakeShortFileName = "fakeFile"
	FakeHashId        = FakeNamespace + "." + FakeController + "." + FakeInstance + "." + FakeFileName
)

func TestMetadata(t *testing.T) {
	tests := []struct{
		name string
		entry api.Entry
		expectedError bool
		expectedNamespace string
		expectedController string
		expectedInstance string
		expectedFilename string
		expectedKubernetes bool
	}{
		{
			name: "nil entry",
			expectedError: true,
		},
		{
			name: "no label",
			entry: api.Entry{},
			expectedError: true,
		},
		{
			name: "full labels",
			entry: api.Entry{Labels: model.LabelSet{NamespaceLabel: FakeNamespace, ControllerNameLabel: FakeController, InstanceLabel: FakeInstance, FileNameLabel: FakeFileName}},
			expectedNamespace: FakeNamespace,
			expectedController: FakeController,
			expectedInstance: FakeInstance,
			expectedFilename: FakeFileName,
			expectedKubernetes: true,
			expectedError: false,
		},
		{
			name: "lack of namespace label",
			entry: api.Entry{Labels: model.LabelSet{ControllerNameLabel: FakeController, InstanceLabel: FakeInstance, FileNameLabel: FakeFileName}},
			expectedNamespace: defaultNamespace,
			expectedController: FakeController,
			expectedInstance: FakeInstance,
			expectedFilename: FakeFileName,
			expectedKubernetes: false,
			expectedError: false,
		},
		{
			name: "lack of instance label",
			entry: api.Entry{Labels: model.LabelSet{NamespaceLabel: FakeNamespace, ControllerNameLabel: FakeController, FileNameLabel: FakeFileName}},
			expectedNamespace: FakeNamespace,
			expectedController: FakeController,
			expectedInstance: defaultInstanceName,
			expectedFilename: FakeFileName,
			expectedKubernetes: true,
			expectedError: false,
		},
		{
			name: "lack of controller label",
			entry: api.Entry{Labels: model.LabelSet{NamespaceLabel: FakeNamespace, InstanceLabel: FakeInstance, FileNameLabel: FakeFileName}},
			expectedNamespace: FakeNamespace,
			expectedController: defaultControllerName,
			expectedInstance: FakeInstance,
			expectedFilename: FakeFileName,
			expectedKubernetes: false,
			expectedError: false,
		},
	}

	for _, tt := range tests{
		t.Run(tt.name, func(t *testing.T) {
			got := Get()
			entry := tt.entry

			if tt.expectedError{
				require.Error(t, parseEntry(&entry, got))
			}else {
				require.NoError(t, parseEntry(&entry, got))
				assert.Equal(t, tt.expectedNamespace, got.namespace)
				assert.Equal(t, tt.expectedController, got.controllerName)
				assert.Equal(t, tt.expectedInstance, got.instance)
				assert.Equal(t, tt.expectedFilename, got.originFilename)
				assert.Equal(t, tt.expectedKubernetes, got.isKubernetes)
			}
			Put(got)
		})
	}


}

func TestGetDeploymentName(t *testing.T) {
	controller := "dmo-metadata-web-per-deploy-5d485fc4b9"
	except := "dmo-metadata-web-per-deploy"
	assert.Equal(t, DeploymentName(controller), except)
}


type FakeFileHandler struct {
	//manager  *Manager
	receiver chan string
	count    int
}
func (f *FakeFileHandler) Status() bool {
	return true
}
func (f *FakeFileHandler) run() {
	for c := range f.receiver {
		_ = c
		f.count++
	}
}
func (f *FakeFileHandler) Receiver() chan string {
	return nil
}
func (f *FakeFileHandler) Stop() {
}