package kafka

import (
	kafka "github.com/Shopify/sarama"
	"github.com/grafana/loki/clients/pkg/promtail/api"
	"github.com/grafana/loki/pkg/logproto"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
	"time"
)

const (
	defaultFakeHostname string= "fake"
)


var testCases = []struct{
	expectedTopicKind TopicKind
	expectedWrapper bool
	expectedEntriesNumber int
	apiEntry api.Entry
	expectKafkaMessage kafka.ProducerMessage
}{
	{
		expectedTopicKind: TopicKindAccessMongo,
		expectedWrapper: true,
		apiEntry: api.Entry{Labels: model.LabelSet{FilenameLabel: model.LabelValue(randString(TopicKindAccessMongo))},
			Entry: logproto.Entry{Timestamp: time.Unix(1, 0).UTC(), Line: "line1"}},
		expectKafkaMessage: kafka.ProducerMessage{
			Topic:     TopicKindAccessMongo.topic(),
			Key:       kafka.ByteEncoder("fakeControllerName"),
			Value:     nil,
			Timestamp: time.Unix(1,0).UTC(),
		},
	},
	{
		expectedTopicKind: TopicKindAccessDubbo,
		expectedWrapper: true,
		apiEntry: api.Entry{Labels: model.LabelSet{FilenameLabel: model.LabelValue(randString(TopicKindAccessDubbo))},
			Entry: logproto.Entry{Timestamp: time.Unix(2,0).UTC(),Line: "line2"}},
		expectKafkaMessage: kafka.ProducerMessage{
			Topic:     TopicKindAccessDubbo.topic(),
			Key:       kafka.ByteEncoder("fakeControllerName"),
			Timestamp: time.Unix(2,0).UTC(),
		},
	},
	{
		expectedTopicKind: TopicKindAccessRest,
		expectedWrapper: true,
		apiEntry: api.Entry{Labels: model.LabelSet{FilenameLabel: model.LabelValue(randString(TopicKindAccessRest))},
			Entry: logproto.Entry{Timestamp: time.Unix(3,0).UTC(), Line: "line3"}},
		expectKafkaMessage: kafka.ProducerMessage{
			Topic:     TopicKindAccessRest.topic(),
			Key:       kafka.ByteEncoder("fakeControllerName"),
			Timestamp: time.Unix(3,0).UTC(),
		},
	},
	{
		expectedTopicKind: TopicKindAppJson,
		apiEntry: api.Entry{Labels: model.LabelSet{FilenameLabel: model.LabelValue(randString(TopicKindAppJson))},
			Entry: logproto.Entry{Timestamp: time.Unix(4,0).UTC(), Line: "line4"}},
		expectedWrapper: false,
		expectKafkaMessage: kafka.ProducerMessage{
			Timestamp: time.Unix(4,0).UTC(),
			Value: kafka.ByteEncoder([]byte("line4")),
			Topic: TopicKindAppJson.topic(),
			Key: kafka.ByteEncoder("fakeControllerName")},
	},
	{
		expectedTopicKind: TopicKindJavaMemory,
		expectedWrapper: true,
		apiEntry: api.Entry{Labels: model.LabelSet{FilenameLabel: model.LabelValue(randString(TopicKindJavaMemory))},
			Entry: logproto.Entry{Timestamp: time.Unix(5,0).UTC(), Line: "line5"}},
		expectKafkaMessage: kafka.ProducerMessage{
			Topic:     TopicKindJavaMemory.topic(),
			Key:       kafka.ByteEncoder("fakeControllerName"),
			Timestamp: time.Unix(5,0).UTC(),
		},
	},
	{
		expectedTopicKind: TopicKindStackDubbo,
		expectedWrapper: true,
		apiEntry: api.Entry{Labels: model.LabelSet{FilenameLabel: model.LabelValue(randString(TopicKindStackDubbo))},
			Entry: logproto.Entry{Timestamp: time.Unix(6,0).UTC(), Line: "line6"}},
	},
	{
		expectedTopicKind: TopicKindGc,
		expectedWrapper: false,
		apiEntry: api.Entry{Labels: model.LabelSet{FilenameLabel: model.LabelValue(randString(TopicKindGc))},
			Entry: logproto.Entry{Timestamp: time.Unix(7, 0).UTC(), Line: "line7"}},
	},
	{
		expectedTopicKind: TopicKindSysDmesg,
		expectedWrapper: true,
		apiEntry: api.Entry{Labels: model.LabelSet{FilenameLabel: model.LabelValue(randString(TopicKindSysDmesg))},
			Entry: logproto.Entry{Timestamp: time.Unix(8, 0).UTC(), Line: "line8"}},
	},
}

func TestBatch_entrysize(t *testing.T){
	t.Parallel()

}

func TestBatch_getTopic(t *testing.T){
	t.Parallel()
	for _, testCase := range testCases{
		t.Run(string(testCase.expectedTopicKind), func(t *testing.T) {
			actualTopic, actualWrapFlag := getTopicKindFromEntry(&testCase.apiEntry)
			assert.Equal(t, testCase.expectedWrapper, actualWrapFlag)
			assert.Equal(t, testCase.expectedTopicKind, actualTopic)
		})
	}
}

// TestBatch_add test add method in batch
func TestBatch_add(t *testing.T){
	batch := newBatch()

	for _, testCase := range testCases{
		batch.add(testCase.apiEntry)
	}
	assert.Equal(t, batch.numEntries, 8)
	assert.Equal(t, batch.sizeBytes(), 11 * 8)
	assert.Equal(t, len(batch.kafkaStreams), 8)
}


func TestBatch_encode(t *testing.T){

}


func TestBatch_entryToKafka(t *testing.T){

}



var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randString(topicKind TopicKind) string {
	b := make([]rune, 10)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b) + string(topicKind) + string(b)
}



func generateTestEntry(topic TopicKind, rand int)[]api.Entry{
	entries := []api.Entry{}
	for _i := 0; _i < rand ; _i ++{
		entries = append(entries, api.Entry{Labels: model.LabelSet{FilenameLabel: model.LabelValue(randString(topic))},
			Entry: logproto.Entry{
				Timestamp: time.Now(),
			}})
	}
	return entries
}