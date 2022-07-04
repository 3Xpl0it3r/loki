package filesystem

import (
	"bufio"
	"context"
	"fmt"


	"github.com/go-kit/log"
	"github.com/grafana/loki/clients/pkg/promtail/api"
	"github.com/grafana/loki/pkg/logproto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"k8s.io/klog"
	"os"
	"path"
	"strconv"
	"sync"
	"testing"
	"time"
)

type FakeClient struct {
	cfg     FileClientConfig
	reg     prometheus.Registerer
	logger  log.Logger
	entries chan api.Entry // 接收来自client的entry

	ctx    context.Context
	cancel context.CancelFunc

	wg   sync.WaitGroup
	once sync.Once

	manager *Manager
}

func TestClientHandle(t *testing.T) {
	var (
		EntryNumber    int = 10000
		TestLabelsKind int = 2
	)
	client := &client{
		cfg:     fakeCfg,

		logger:  nil,
		entries: nil,
		ctx:     nil,
		cancel:  nil,
		wg:      sync.WaitGroup{},
		once:    sync.Once{},
		manager: nil,
	}
	cloneModel := func(fileName string) model.LabelSet {
		return model.LabelSet{NamespaceLabel: FakeNamespace, ControllerNameLabel: FakeController, InstanceLabel: FakeInstance, FileNameLabel: model.LabelValue(fileName)}
	}
	klog.SetOutput(os.Stderr)

	client, err := NewFileSystemClient(nil, FileClientConfig{Path: "./tmp"}, nil, log.NewNopLogger())
	require.NoError(t, err)

	for i := 0; i < EntryNumber; i++ {
		title := "file" + strconv.Itoa(i%TestLabelsKind)
		client.Chan() <- api.Entry{Labels: cloneModel(title),
			Entry: logproto.Entry{Timestamp: time.Time{}, Line: fmt.Sprintf("line %d for %s", i, title)}}
	}
	require.Equal(t, client.manager.HandlerSize(), TestLabelsKind)
	client.Stop()

	for i := 0; i < TestLabelsKind; i++ {
		title := "file" + strconv.Itoa(i)
		root := "tmp/fakeNamespace/fakeController/" + date2String() + "/fakeInstance"
		fp, err := os.Open(path.Join(root, title))
		require.NoError(t, err)
		scanner := bufio.NewScanner(fp)
		var count int = 0
		for scanner.Scan() {
			count++
		}
		require.Equal(t, EntryNumber/TestLabelsKind, count)
		require.NoError(t, fp.Close())
	}

	require.NoError(t, os.RemoveAll(fakeCfg.Path))
}

func _createFakeFileHandler() Handler {
	return nil
}
