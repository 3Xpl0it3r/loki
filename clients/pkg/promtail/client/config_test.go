package client

import (
	"testing"
)

var clientConfig = Configs{}

var clientDefaultConfig = (`
url: http://localhost:3100/loki/api/v1/push
kind: loki
`)

var clientCustomLokiConfig = `
url: http://localhost:3100/loki/api/v1/push
kind: loki
backoff_config:
  max_retries: 20
  min_period: 5s
  max_period: 1m
batchwait: 5s
batchsize: 204800
timeout: 5s
`

var clientMultiCustomEsConfig = `
`

func Test_Config(t *testing.T) {
	//u, err := url.Parse("http://localhost:3100/loki/api/v1/push")
	//require.NoError(t, err)
	//tests := []struct {
	//	configValues   string
	//	expectedConfig Config
	//}{
	//	{
	//		clientCustomLokiConfig,
	//		Config{
	//			Kind: LokiClient,
	//			LokiConfig: loki.LokiConfig{
	//				URL:       flagext.URLValue{URL: u},
	//				BatchWait: 5 * time.Second,
	//				BatchSize: 100 * 2048,
	//				BackoffConfig: util.BackoffConfig{
	//					MaxRetries: 20,
	//					MaxBackoff: 1 * time.Minute,
	//					MinBackoff: 5 * time.Second,
	//				},
	//				ExternalLabels: fe.LabelSet{},
	//				Timeout:        5 * time.Second,
	//			},
	//		},
	//	},
	//}
	//for _, tc := range tests {
	//	err := yaml.Unmarshal([]byte(tc.configValues), &clientConfig)
	//	require.NoError(t, err)
	//
	//	if !reflect.DeepEqual(tc.expectedConfig, clientConfig) {
	//		t.Errorf("Configs does not match, expected: %v, received: %v", tc.expectedConfig, clientConfig)
	//	}
	//}
}
