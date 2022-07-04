package client

import (
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/grafana/loki/clients/pkg/promtail/api"

	"github.com/grafana/loki/pkg/logproto"
	util_log "github.com/grafana/loki/pkg/util/log"
)

func TestNewLogger(t *testing.T) {
	_, err := NewLogger(nil, nil, util_log.Logger, []Config{}...)
	require.Error(t, err)

	l, err := NewLogger(nil, nil, util_log.Logger)
	require.NoError(t, err)
	l.Chan() <- api.Entry{Labels: model.LabelSet{"foo": "bar"}, Entry: logproto.Entry{Timestamp: time.Now(), Line: "entry"}}
	l.Stop()
}
