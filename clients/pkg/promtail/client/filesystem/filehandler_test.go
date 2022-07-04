package filesystem

import (
	"bufio"
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path"
	"testing"
	"time"
)

var (
	fakeMt = metadata{
		namespace:      FakeNamespace,
		controllerName: FakeController,
		instance:       FakeInstance,
		fileName:       FakeShortFileName,
		originFilename: FakeFileName,
		isKubernetes:   false,
	}

	fakeCfg = FileClientConfig{Path: "tmp", ReSyncPeriod: 1}
)

func Test_FileHandler_Handler(t *testing.T) {
	cases := []struct {
		name               string
		handlerId          string
		entries            []string
		manager            *Manager
		expectedReceiveNum int
	}{
		{
			name:               "none manager, no line",
			handlerId:          "file1",
			entries:            []string{},
			manager:            nil,
			expectedReceiveNum: 0,
		},
		{
			name:               "none manager, 4 lines",
			handlerId:          "file2",
			entries:            []string{"line1", "line2", "line3", "line4"},
			manager:            nil,
			expectedReceiveNum: 4,
		},
		{
			name:               "with manager, no line",
			handlerId:          "file3",
			entries:            []string{},
			manager:            newHandlerManager(),
			expectedReceiveNum: 0,
		},
		{
			name:               "with manager, 4 line",
			handlerId:          "file4",
			entries:            []string{"line1", "line2", "line3", "line4"},
			manager:            newHandlerManager(),
			expectedReceiveNum: 4,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := newFileHandler(context.Background(), nil, &fakeCfg, tt.manager, &fakeMt)
			require.NoError(t, err)
			for _, line := range tt.entries {
				handler.Receiver() <- line
			}
			handler.Stop()

			fileName := path.Join(handler.directoryName, handler.writeFileName)
			f, err := os.Open(fileName)
			require.NoError(t, err)
			scanner := bufio.NewScanner(f)
			var count int = 0
			for scanner.Scan() {
				require.Equal(t, scanner.Text(), tt.entries[count])
				count++
			}

			require.Equal(t, tt.expectedReceiveNum, count)
			require.NoError(t, f.Close())
			require.NoError(t, os.Remove(fileName))
			require.NoError(t, os.RemoveAll(handler.directoryName))
			require.NoError(t, os.RemoveAll(fakeCfg.Path))
		})
	}
}

func Test_FileHandler_Rotate(t *testing.T) {
	cases := []struct {
		name              string
		newDate           string
		entries           []string
		isKubernetes      bool
		expectedDirectory string
	}{
		{
			name:              "isKubernetes",
			newDate:           "2022-10-11",
			entries:           []string{"line1", "line2", "line3", "line4"},
			isKubernetes:      true,
			expectedDirectory: "tmp/fakeNamespace/fakeController/2022-10-11/fakeInstance/fakeFile",
		},
		{
			name:              "file log based on file-sd-discovery",
			newDate:           "2022-10-11",
			entries:           []string{"line1", "line2", "line3", "line4"},
			isKubernetes:      false,
			expectedDirectory: "tmp/fakeNamespace/fakeController/fakeInstance/fakeDir1/fakeDir2/fakeFile",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			copyMt := fakeMt
			copyMt.isKubernetes = tt.isKubernetes
			handler, err := newFileHandler(context.Background(), nil, &fakeCfg, nil, &copyMt)
			assert.NoError(t, err)
			count := 0
			for ; count < len(tt.entries)/2; count++ {
				handler.Receiver() <- tt.entries[count]
			}
			oldFilename := handler.directoryName + "/" + handler.writeFileName
			handler.rotateCh <- tt.newDate
			time.Sleep(fakeCfg.ReSyncPeriod)
			_validateFileContent(t, oldFilename, tt.entries[:(len(tt.entries)/2)], len(tt.entries)>>1)

			for ; count < len(tt.entries); count++ {
				handler.Receiver() <- tt.entries[count]
			}
			newFilename := path.Join(handler.directoryName, handler.writeFileName)
			handler.Stop()

			require.Equal(t, tt.expectedDirectory, newFilename)
			//require.Equal(t, count, len(tt.entries) / 2)

			require.Equal(t, tt.expectedDirectory, newFilename)
			if tt.isKubernetes {
				_validateFileContent(t, newFilename, tt.entries[(len(tt.entries)/2):], len(tt.entries)>>1)
			} else {
				_validateFileContent(t, newFilename, tt.entries, len(tt.entries))
			}

			require.NoError(t, os.RemoveAll(fakeCfg.Path))
		})
	}
}

func Test_FileHandler_FileWatcher(t *testing.T) {
	cases := []struct {
		name             string
		entries          []string
		isKubernetes     bool
		expectedCount    int
	}{
		{
			name:    "renamed file, is kubernetes",
			entries: []string{"line1", "line2", "line3", "line4"},
			expectedCount: 2,
			isKubernetes:  true,
		},
		{
			name:    "removed file, is kubernetes",
			entries: []string{"line1", "line2", "line3", "line4"},
			expectedCount: 2,
			isKubernetes:  true,
		},
		{
			name:         "renamed file, file discovery",
			entries:      []string{"line1", "line2", "line3", "line4"},
			isKubernetes: false,
		},
		{
			name:         "removed file, file discovery",
			entries:      []string{"line1", "line2", "line3", "line4"},
			isKubernetes: false,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			copyMt := fakeMt
			copyMt.isKubernetes = tt.isKubernetes
			handler, err := newFileHandler(context.Background(), nil, &fakeCfg, nil, &copyMt)
			assert.NoError(t, err)
			count := 0
			for ; count < len(tt.entries)/2; count++ {
				handler.Receiver() <- tt.entries[count]
			}
			oldFileName := path.Join(handler.directoryName, handler.writeFileName)

			synced := false
			for i := 0; i < 5; i++ {
				if len(handler.batchBuffer.streams) == 0 {
					synced = true
					break
				}
				time.Sleep(1 * time.Second)
			}
			require.Equal(t, true, synced)

			_validateFileContent(t, oldFileName, tt.entries[:count], len(tt.entries)/2)
			// remove file, triger filewatcher
			require.NoError(t, os.Remove(oldFileName))

			synced = false
			for i := 0; i < 5; i++ {
				if len(handler.batchBuffer.streams) == 0 {
					synced = true
					break
				}
				time.Sleep(1 * time.Second)
			}
			require.Equal(t, true, synced)
			time.Sleep(2 * time.Second)
			for ; count < len(tt.entries); count++ {
				handler.Receiver() <- tt.entries[count]
			}
			handler.Stop()
			_validateFileContent(t, oldFileName, tt.entries[len(tt.entries)>>1:], 2)
			require.NoError(t, os.RemoveAll(fakeCfg.Path))
		})
	}
}

func _validateFileContent(t *testing.T, fileName string, content []string, expectedReadLines int) {

	fp, err := os.Open(fileName)
	require.NoError(t, err)
	scanner := bufio.NewScanner(fp)
	count := 0
	for scanner.Scan() {
		require.Equal(t, content[count],scanner.Text())
		count++
	}
	require.Equal(t, expectedReadLines, count)
	require.NoError(t, fp.Close())
}
