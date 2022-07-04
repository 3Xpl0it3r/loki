package filesystem

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_Config_defaulter(t *testing.T){
	cfg := DefaultFileSystemConfig()
	assert.Equal(t, cfg.BatchSize, DefaultBatchSize)
	assert.Equal(t, cfg.BatchWait, DefaultBatchWait)
	assert.Equal(t, cfg.Timeout, DefaultTimeout)
	assert.Equal(t, cfg.ReSyncPeriod, DefaultFileHandlerReSyncPeriod)
}