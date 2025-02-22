package kafka

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	kafka "github.com/Shopify/sarama"
	"github.com/grafana/loki/clients/pkg/promtail/api"
	"github.com/prometheus/common/model"
)

const (
	ControllerNameLabel model.LabelName = "controller_name"
)

var (
	DefaultHostName string
	once            sync.Once
)

func init() {
	once.Do(func() {
		DefaultHostName, _ = os.Hostname()
	})
}

type TopicKind string

const (
	// jsonApp/raw
	TopicKindAccessMongo TopicKind = "sql"
	TopicKindAccessDubbo TopicKind = "dubboAccess"
	TopicKindAccessRest  TopicKind = "access"
	TopicKindGc          TopicKind = "gc.log"
	TopicKindJavaMemory  TopicKind = "memory.log"
	TopicKindStackDubbo  TopicKind = "DubboStack"

	// common log
	TopicKindAppJson TopicKind = "jsonApp" // buf
	TopicKindUnknown TopicKind = "promtail-unknown"

	TopicKindIpaasApp TopicKind = "app_json_logs/application"

	// system log
	TopicKindSysDmesg TopicKind = "dmesg"

	TopicKindFake TopicKind = "fake.log"

	TopicKindDockerStdout TopicKind = "-json.log"
)

func (t TopicKind) String() string {
	switch t {
	case TopicKindAccessMongo:
		return "promtail-accessmongo"
	case TopicKindAccessDubbo:
		return "promtail-accessdubbo"
	case TopicKindStackDubbo:
		return "promtail-dubbo-stack"
	case TopicKindAccessRest:
		return "promtail-accessrest"
	case TopicKindGc:
		return "promtail-gc"
	case TopicKindJavaMemory:
		return "promtail-memory"
	case TopicKindAppJson:
		return "promtail-app"
	case TopicKindFake:
		return "promtail-fake"
	case TopicKindSysDmesg:
		return "promtail-sysdmesg"
	case TopicKindIpaasApp:
		return "promtail-ipaas"
	case TopicKindDockerStdout:
		return "promtail-docker"
	default:
		return "promtail-known"
	}
}

//  getTopicKindFromEntry
//  用来区分topic 以及 topic类型
func getTopicKindFromEntry(e *api.Entry) (topKind TopicKind, shouldWrapJson bool) {
	filenameLabelValue, ok := e.Labels[FilenameLabel]
	if !ok {
		topKind, shouldWrapJson = TopicKindUnknown, false
		return
	}

	filenameSlice := strings.Split(string(filenameLabelValue), "/")
	filename := filenameSlice[len(filenameSlice)-1]
	if strings.Contains(filename, "localhost_access_log") {
		topKind, shouldWrapJson = TopicKindUnknown, true
		return
	}

	if strings.HasSuffix(filename, string(TopicKindDockerStdout)) {
		return TopicKindDockerStdout, true
	}

	if strings.Contains(string(filenameLabelValue), string(TopicKindIpaasApp)) {
		return TopicKindIpaasApp, false
	}

	if strings.Contains(filename, string(TopicKindAccessMongo)) {
		topKind, shouldWrapJson = TopicKindAccessMongo, true
	} else if strings.Contains(filename, string(TopicKindAccessDubbo)) {
		topKind, shouldWrapJson = TopicKindAccessDubbo, true
	} else if strings.Contains(filename, string(TopicKindAccessRest)) {
		topKind, shouldWrapJson = TopicKindAccessRest, true
	} else if strings.Contains(filename, string(TopicKindGc)) {
		topKind, shouldWrapJson = TopicKindGc, true
	} else if strings.Contains(filename, string(TopicKindJavaMemory)) {
		topKind, shouldWrapJson = TopicKindJavaMemory, true
	} else if strings.Contains(filename, string(TopicKindAppJson)) { // appJson is not common
		topKind, shouldWrapJson = TopicKindAppJson, true
	} else if strings.Contains(filename, string(TopicKindFake)) {
		topKind, shouldWrapJson = TopicKindFake, true
	} else if strings.Contains(filename, string(TopicKindSysDmesg)) {
		topKind, shouldWrapJson = TopicKindSysDmesg, true
	} else if strings.Contains(filename, string(TopicKindStackDubbo)) {
		topKind, shouldWrapJson = TopicKindStackDubbo, true
	} else {
		topKind, shouldWrapJson = TopicKindUnknown, true
	}
	return
}

const (
	MaxLogSize = 1024 * 1024
)

type kafkaStream struct {
	Messages []*kafka.ProducerMessage
}

// batch holds pending log streams waiting to be sent to Loki, and it's used
// to reduce the number of push requests to Loki aggregating multiple log streams
// and entries in a single batch request. In case of multi-tenant Promtail, log
// streams for each tenant are stored in a dedicated batch.
type batch struct {
	//streams      map[string]*logproto.Stream
	bytes        int
	createdAt    time.Time
	kafkaStreams map[string]*kafkaStream
	numEntries   int
}

func newBatch(entries ...api.Entry) *batch {
	b := &batch{
		//streams:      map[string]*logproto.Stream{},
		kafkaStreams: map[string]*kafkaStream{},
		bytes:        0,
		createdAt:    time.Now(),
		numEntries:   0,
	}

	// Add entries to the batch
	for _, entry := range entries {
		b.add(entry)
	}
	return b
}

// add an entry to the batch
func (b *batch) add(entry api.Entry) {
	// unknown topicKind will be dropped
	topicKind, shouldWrapJson := getTopicKindFromEntry(&entry)
	if topicKind == TopicKindUnknown {
		return
	}
	b.bytes += entry.Size()
	b.numEntries += 1
	// Append the entry to an already existing stream (if any)
	labels := labelsMapToString(entry.Labels, ReservedLabelTenantID)
	if streams, ok := b.kafkaStreams[labels]; ok {
		streams.Messages = append(streams.Messages, entryToKafkaMessage(entry, topicKind, shouldWrapJson))
		return
	}

	// Add kafka message as new message
	b.kafkaStreams[labels] = &kafkaStream{Messages: []*kafka.ProducerMessage{entryToKafkaMessage(entry, topicKind, shouldWrapJson)}}
}

func labelsMapToString(ls model.LabelSet, without ...model.LabelName) string {
	lstrs := make([]string, 0, len(ls))
Outer:
	for l, v := range ls {
		for _, w := range without {
			if l == w {
				continue Outer
			}
		}
		lstrs = append(lstrs, fmt.Sprintf("%s=%q", l, v))
	}

	sort.Strings(lstrs)
	return fmt.Sprintf("{%s}", strings.Join(lstrs, ", "))
}

// sizeBytes returns the current batch size in bytes
//go: inline
func (b *batch) sizeBytes() int {
	return b.bytes
}

// sizeBytesAfter returns the size of the batch after the input entry
// will be added to the batch itself
func (b *batch) sizeBytesAfter(entry api.Entry) int {
	return b.bytes + entry.Size()
}

// age of the batch since its creation
func (b *batch) age() time.Duration {
	return time.Since(b.createdAt)
}

// encode the batch as as kafka messages
// the encoded bytes and the number of encoded entries
func (b *batch) encode() ([]*kafka.ProducerMessage, int, error) {
	var (
		requests     []*kafka.ProducerMessage = make([]*kafka.ProducerMessage, 0, b.numEntries)
		entriesCount int                      = 0
	)
	for _, streams := range b.kafkaStreams {
		requests = append(requests, streams.Messages...)
		entriesCount += len(streams.Messages)
	}
	return requests, entriesCount, nil
}

//go:inline
func entryToKafkaMessage(e api.Entry, topKind TopicKind, shouldWrapJson bool) *kafka.ProducerMessage {
	req := e.Labels.Merge(map[model.LabelName]model.LabelValue{
		"timestamp": model.LabelValue(e.Timestamp.String()),
		"message":   model.LabelValue(e.Line),
		"hostname":  model.LabelValue(DefaultHostName),
	})

	var value []byte
	if shouldWrapJson == true {
		value, _ = json.Marshal(req)
	} else {
		value = []byte(e.Line)
	}

	return &kafka.ProducerMessage{
		Timestamp: e.Timestamp,
		Value:     kafka.ByteEncoder(value),
		Topic:     topKind.String(),
		Key:       kafka.ByteEncoder(deploymentNameFromEntry(&e)),
	}
}

// DeploymentName get deployment from controllerName
// in common case controllerName is represent the name of replicaset
func deploymentNameFromEntry(e *api.Entry) string {
	var controllerName string
	if value, ok := e.Labels[ControllerNameLabel]; ok {
		controllerName = string(value)
	} else {
		controllerName = "fakeControllerName"
	}
	nSplit := strings.Split(controllerName, "-")
	if len(nSplit) >= 2 {
		return strings.Join(nSplit[:len(nSplit)-1], "-")
	}
	return controllerName
}

func (b *batch) getNumEntries() int {
	return b.numEntries
}

func (b *batch) free() {
	b.numEntries = 0
	b.bytes = 0
	b.kafkaStreams = map[string]*kafkaStream{}
}
