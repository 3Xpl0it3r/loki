package client

import (
	"fmt"
	clientmetrics "github.com/grafana/loki/clients/pkg/promtail/client/metrics"
	"os"
	"runtime"
	"sync"
	"text/tabwriter"

	"github.com/fatih/color"
	"github.com/go-kit/log"
	"gopkg.in/yaml.v2"

	"github.com/grafana/loki/clients/pkg/promtail/api"
)

var (
	yellow = color.New(color.FgYellow)
	blue   = color.New(color.FgBlue)
)

func init() {
	if runtime.GOOS == "windows" {
		yellow.DisableColor()
		blue.DisableColor()
	}
}

type logger struct {
	*tabwriter.Writer
	sync.Mutex
	entries chan api.Entry

	once sync.Once
}

// NewLogger creates a new client logger that logs entries instead of sending them.
func NewLogger(metrics *clientmetrics.Metrics, streamLogLabels []string, log log.Logger, cfgs Configs) (Client, error) {
	// make sure the clients config is valid
	c, err := NewMulti(metrics, streamLogLabels, log, cfgs)
	if err != nil {
		return nil, err
	}
	c.Stop()

	fmt.Println(yellow.Sprint("Clients configured:"))

	yaml, err := yaml.Marshal(cfgs)
	if err != nil {
		return nil, err
	}
	fmt.Println("----------------------")
	fmt.Println(string(yaml))
	entries := make(chan api.Entry)
	l := &logger{
		Writer:  tabwriter.NewWriter(os.Stdout, 0, 8, 0, '\t', 0),
		entries: entries,
	}
	go l.run()
	return l, nil
}

func (l *logger) Stop() {
	l.once.Do(func() { close(l.entries) })
}

func (l *logger) Chan() chan<- api.Entry {
	return l.entries
}

func (l *logger) run() {
	for e := range l.entries {
		fmt.Fprint(l.Writer, blue.Sprint(e.Timestamp.Format("2006-01-02T15:04:05.999999999-0700")))
		fmt.Fprint(l.Writer, "\t")
		fmt.Fprint(l.Writer, yellow.Sprint(e.Labels.String()))
		fmt.Fprint(l.Writer, "\t")
		fmt.Fprint(l.Writer, e.Line)
		fmt.Fprint(l.Writer, "\n")
		l.Flush()
	}
}
func (l *logger) StopNow() { l.Stop() }

func (l *logger) Name() string {
	return ""
}
