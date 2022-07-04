package kafka


type receivedReq struct {
	tenantID string
	pushReq kafkaStream
}

//func TestClient_Handle(t *testing.T) {
//
//	tests := map[string]struct {
//		clientBatchSize      int
//		clientBatchWait      time.Duration
//		clientMaxRetries     int
//		clientTenantID       string
//		serverResponseStatus int
//		inputEntries         []api.Entry
//		inputDelay           time.Duration
//		expectedReqs         []receivedReq
//		expectedMetrics      string
//	}{
//		"batch log entries together until the batch size is reached": {
//			clientBatchSize:      10,
//			clientBatchWait:      100 * time.Millisecond,
//			clientMaxRetries:     3,
//			serverResponseStatus: 200,
//			inputEntries:         []api.Entry{logEntries[0], logEntries[1], logEntries[2]},
//			expectedReqs: []receivedReq{
//				{
//					tenantID: "",
//					pushReq: kafkaStream{Messages: []kafka.Message{entryConvertToKafkaMessage(logEntries[0]), entryConvertToKafkaMessage(logEntries[1])}},
//				},
//				{
//					tenantID: "",
//					pushReq:  kafkaStream{Messages: []kafka.Message{entryConvertToKafkaMessage(logEntries[2])}},
//				},
//			},
//			expectedMetrics: `
//				# HELP promtail_sent_entries_total Number of log entries sent to the ingester.
//				# TYPE promtail_sent_entries_total counter
//				promtail_sent_entries_total{host="__HOST__"} 3.0
//				# HELP promtail_dropped_entries_total Number of log entries dropped because failed to be sent to the ingester after all retries.
//				# TYPE promtail_dropped_entries_total counter
//				promtail_dropped_entries_total{host="__HOST__"} 0
//			`,
//		},
//		"batch log entries together until the batch wait time is reached": {
//			clientBatchSize:      10,
//			clientBatchWait:      100 * time.Millisecond,
//			clientMaxRetries:     3,
//			serverResponseStatus: 200,
//			inputEntries:         []api.Entry{logEntries[0], logEntries[1]},
//			inputDelay:           110 * time.Millisecond,
//			expectedReqs: []receivedReq{
//				{
//					tenantID: "",
//					pushReq: kafkaStream{Messages: []kafka.Message{entryConvertToKafkaMessage(logEntries[0])}},
//				},
//				{
//					tenantID: "",
//					pushReq: kafkaStream{Messages: []kafka.Message{entryConvertToKafkaMessage(logEntries[1])}},
//				},
//			},
//			expectedMetrics: `
//				# HELP promtail_sent_entries_total Number of log entries sent to the ingester.
//				# TYPE promtail_sent_entries_total counter
//				promtail_sent_entries_total{host="__HOST__"} 2.0
//				# HELP promtail_dropped_entries_total Number of log entries dropped because failed to be sent to the ingester after all retries.
//				# TYPE promtail_dropped_entries_total counter
//				promtail_dropped_entries_total{host="__HOST__"} 0
//			`,
//		},
//		"retry send a batch up to backoff's max retries in case the server responds with a 5xx": {
//			clientBatchSize:      10,
//			clientBatchWait:      10 * time.Millisecond,
//			clientMaxRetries:     3,
//			serverResponseStatus: 500,
//			inputEntries:         []api.Entry{logEntries[0]},
//			expectedReqs: []receivedReq{
//				{
//					tenantID: "",
//					pushReq:  logproto.PushRequest{Streams: []logproto.Stream{{Labels: "{}", Entries: []logproto.Entry{logEntries[0].Entry}}}},
//				},
//				{
//					tenantID: "",
//					pushReq:  logproto.PushRequest{Streams: []logproto.Stream{{Labels: "{}", Entries: []logproto.Entry{logEntries[0].Entry}}}},
//				},
//				{
//					tenantID: "",
//					pushReq:  logproto.PushRequest{Streams: []logproto.Stream{{Labels: "{}", Entries: []logproto.Entry{logEntries[0].Entry}}}},
//				},
//			},
//			expectedMetrics: `
//				# HELP promtail_dropped_entries_total Number of log entries dropped because failed to be sent to the ingester after all retries.
//				# TYPE promtail_dropped_entries_total counter
//				promtail_dropped_entries_total{host="__HOST__"} 1.0
//				# HELP promtail_sent_entries_total Number of log entries sent to the ingester.
//				# TYPE promtail_sent_entries_total counter
//				promtail_sent_entries_total{host="__HOST__"} 0
//			`,
//		},
//		"do not retry send a batch in case the server responds with a 4xx": {
//			clientBatchSize:      10,
//			clientBatchWait:      10 * time.Millisecond,
//			clientMaxRetries:     3,
//			serverResponseStatus: 400,
//			inputEntries:         []api.Entry{logEntries[0]},
//			expectedReqs: []receivedReq{
//				{
//					tenantID: "",
//					pushReq: kafkaStream{Messages: []kafka.Message{entryConvertToKafkaMessage(logEntries[0])}},
//				},
//			},
//			expectedMetrics: `
//				# HELP promtail_dropped_entries_total Number of log entries dropped because failed to be sent to the ingester after all retries.
//				# TYPE promtail_dropped_entries_total counter
//				promtail_dropped_entries_total{host="__HOST__"} 1.0
//				# HELP promtail_sent_entries_total Number of log entries sent to the ingester.
//				# TYPE promtail_sent_entries_total counter
//				promtail_sent_entries_total{host="__HOST__"} 0
//			`,
//		},
//		"do retry sending a batch in case the server responds with a 429": {
//			clientBatchSize:      10,
//			clientBatchWait:      10 * time.Millisecond,
//			clientMaxRetries:     3,
//			serverResponseStatus: 429,
//			inputEntries:         []api.Entry{logEntries[0]},
//			expectedReqs: []receivedReq{
//				{
//					tenantID: "",
//					pushReq: kafkaStream{Messages: []kafka.Message{entryConvertToKafkaMessage(logEntries[0])}},
//				},
//				{
//					tenantID: "",
//					pushReq: kafkaStream{Messages: []kafka.Message{entryConvertToKafkaMessage(logEntries[0])}},
//				},
//				{
//					tenantID: "",
//					pushReq: kafkaStream{Messages: []kafka.Message{entryConvertToKafkaMessage(logEntries[0])}},
//				},
//			},
//			expectedMetrics: `
//				# HELP promtail_dropped_entries_total Number of log entries dropped because failed to be sent to the ingester after all retries.
//				# TYPE promtail_dropped_entries_total counter
//				promtail_dropped_entries_total{host="__HOST__"} 1.0
//				# HELP promtail_sent_entries_total Number of log entries sent to the ingester.
//				# TYPE promtail_sent_entries_total counter
//				promtail_sent_entries_total{host="__HOST__"} 0
//			`,
//		},
//		"batch log entries together honoring the client tenant ID": {
//			clientBatchSize:      100,
//			clientBatchWait:      100 * time.Millisecond,
//			clientMaxRetries:     3,
//			clientTenantID:       "tenant-default",
//			serverResponseStatus: 200,
//			inputEntries:         []api.Entry{logEntries[0], logEntries[1]},
//			expectedReqs: []receivedReq{
//				{
//					tenantID: "tenant-default",
//					pushReq: kafkaStream{Messages: []kafka.Message{entryConvertToKafkaMessage(logEntries[0]), entryConvertToKafkaMessage(logEntries[1])}},
//				},
//			},
//			expectedMetrics: `
//				# HELP promtail_sent_entries_total Number of log entries sent to the ingester.
//				# TYPE promtail_sent_entries_total counter
//				promtail_sent_entries_total{host="__HOST__"} 2.0
//				# HELP promtail_dropped_entries_total Number of log entries dropped because failed to be sent to the ingester after all retries.
//				# TYPE promtail_dropped_entries_total counter
//				promtail_dropped_entries_total{host="__HOST__"} 0
//			`,
//		},
//		"batch log entries together honoring the tenant ID overridden while processing the pipeline stages": {
//			clientBatchSize:      100,
//			clientBatchWait:      100 * time.Millisecond,
//			clientMaxRetries:     3,
//			clientTenantID:       "tenant-default",
//			serverResponseStatus: 200,
//			inputEntries:         []api.Entry{logEntries[0], logEntries[3], logEntries[4], logEntries[5]},
//			expectedReqs: []receivedReq{
//				{
//					tenantID: "tenant-default",
//					pushReq: kafkaStream{Messages: []kafka.Message{entryConvertToKafkaMessage(logEntries[0])}},
//					//pushReq:  logproto.PushRequest{Streams: []logproto.Stream{{Labels: "{}", Entries: []logproto.Entry{logEntries[0].Entry}}}},
//				},
//				{
//					tenantID: "tenant-1",
//					pushReq: kafkaStream{Messages: []kafka.Message{entryConvertToKafkaMessage(logEntries[3]), entryConvertToKafkaMessage(logEntries[4])}},
//				},
//				{
//					tenantID: "tenant-2",
//					pushReq: kafkaStream{Messages: []kafka.Message{entryConvertToKafkaMessage(logEntries[5])}},
//				},
//			},
//			expectedMetrics: `
//				# HELP promtail_sent_entries_total Number of log entries sent to the ingester.
//				# TYPE promtail_sent_entries_total counter
//				promtail_sent_entries_total{host="__HOST__"} 4.0
//				# HELP promtail_dropped_entries_total Number of log entries dropped because failed to be sent to the ingester after all retries.
//				# TYPE promtail_dropped_entries_total counter
//				promtail_dropped_entries_total{host="__HOST__"} 0
//			`,
//		},
//	}
//
//	for testName, testData := range tests {
//		t.Run(testName, func(t *testing.T) {
//			reg := prometheus.NewRegistry()
//
//			// Create a buffer channel where we do enqueue received requests
//			receivedReqsChan := make(chan receivedReq, 10)
//
//			// Start a local HTTP server
//			server := httptest.NewServer(createServerHandler(receivedReqsChan, testData.serverResponseStatus))
//			require.NotNil(t, server)
//			defer server.Close()
//
//			// Get the URL at which the local test server is listening to
//			serverURL := flagext.URLValue{}
//			err := serverURL.Set(server.URL)
//			require.NoError(t, err)
//
//			// Instance the client
//			cfg := KafkaConfig{
//				Url:            "",
//				Protocol:       "",
//				Topic:          "",
//				GroupId:        "",
//				Partition:      0,
//				BatchWait:      0,
//				BatchSize:      0,
//				BackoffConfig:  util.BackoffConfig{},
//				ExternalLabels: lokiflag.LabelSet{},
//				Timeout:        0,
//			}
//
//			c,err := NewKafkaClient(reg, cfg, log.NewNopLogger())
//			require.NoError(t, err)
//
//			// Send all the input log entries
//			for i, logEntry := range testData.inputEntries {
//				c.Chan() <- logEntry
//
//				if testData.inputDelay > 0 && i < len(testData.inputEntries)-1 {
//					time.Sleep(testData.inputDelay)
//				}
//			}
//
//			// Wait until the expected push requests are received (with a timeout)
//			deadline := time.Now().Add(1 * time.Second)
//			for len(receivedReqsChan) < len(testData.expectedReqs) && time.Now().Before(deadline) {
//				time.Sleep(5 * time.Millisecond)
//			}
//
//			// Stop the client: it waits until the current batch is sent
//			c.Stop()
//			close(receivedReqsChan)
//
//			// Get all push requests received on the server side
//			receivedReqs := make([]receivedReq, 0)
//			for req := range receivedReqsChan {
//				receivedReqs = append(receivedReqs, req)
//			}
//
//			// Due to implementation details (maps iteration ordering is random) we just check
//			// that the expected requests are equal to the received requests, without checking
//			// the exact order which is not guaranteed in case of multi-tenant
//			require.ElementsMatch(t, testData.expectedReqs, receivedReqs)
//
//			//expectedMetrics := strings.Replace(testData.expectedMetrics, "__HOST__", serverURL.Host, -1)
//			//err = testutil.GatherAndCompare(reg, strings.NewReader(expectedMetrics), "promtail_sent_entries_total", "promtail_dropped_entries_total")
//			//assert.NoError(t, err)
//		})
//	}
//}


//func createServerHandler(receivedReqsChan chan receivedReq, status int) http.HandlerFunc {
//	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
//		// Parse the request
//		var pushReq kafkaStream
//		if err := util.ParseProtoReader(req.Context(), req.Body, int(req.ContentLength), math.MaxInt32, &pushReq, util.RawSnappy); err != nil {
//			rw.WriteHeader(500)
//			return
//		}
//
//		receivedReqsChan <- receivedReq{
//			tenantID: req.Header.Get("X-Scope-OrgID"),
//			pushReq:  pushReq,
//		}
//
//		rw.WriteHeader(status)
//	})
//}
