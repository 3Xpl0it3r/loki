package file

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/bmatcuk/doublestar"
	"gopkg.in/fsnotify.v1"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"

	"github.com/grafana/loki/clients/pkg/logentry/stages"
	"github.com/grafana/loki/clients/pkg/promtail/api"
	"github.com/grafana/loki/clients/pkg/promtail/positions"
	"github.com/grafana/loki/clients/pkg/promtail/scrapeconfig"
	"github.com/grafana/loki/clients/pkg/promtail/targets/target"

	cri "github.com/grafana/loki/clients/pkg/promtail/container"
	"github.com/grafana/loki/pkg/util"
)

const (
	pathLabel              = "__path__"
	pathExcludeLabel       = "__path_exclude__"
	hostLabel              = "__host__"
	kubernetesPodNodeField = "spec.nodeName"

	metaLabelPrefix     = model.MetaLabelPrefix + "kubernetes_"
	podContainerIdLabel = metaLabelPrefix + "pod_container_id"
	podNameLabel        = metaLabelPrefix + "pod_name"
	podControllerName   = metaLabelPrefix + "pod_controller_name"
)

// FileTargetManager manages a set of targets.
// nolint:revive
type FileTargetManager struct {
	log     log.Logger
	quit    context.CancelFunc
	syncers map[string]*targetSyncer
	manager *discovery.Manager

	watcher            *fsnotify.Watcher
	targetEventHandler chan fileTargetEvent

	wg sync.WaitGroup
}

// NewFileTargetManager creates a new TargetManager.
func NewFileTargetManager(
	metrics *Metrics,
	logger log.Logger,
	positions positions.Positions,
	client api.EntryHandler,
	scrapeConfigs []scrapeconfig.Config,
	targetConfig *Config,
	criClient cri.Client,
) (*FileTargetManager, error) {
	reg := metrics.reg
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	ctx, quit := context.WithCancel(context.Background())
	tm := &FileTargetManager{
		log:                logger,
		quit:               quit,
		watcher:            watcher,
		targetEventHandler: make(chan fileTargetEvent),
		syncers:            map[string]*targetSyncer{},
		manager:            discovery.NewManager(ctx, log.With(logger, "component", "discovery")),
	}

	hostname, err := hostname()
	if err != nil {
		return nil, err
	}

	configs := map[string]discovery.Configs{}
	for _, cfg := range scrapeConfigs {
		if !cfg.HasServiceDiscoveryConfig() {
			continue
		}

		// stage 组合
		pipeline, err := stages.NewPipeline(log.With(logger, "component", "file_pipeline"), cfg.PipelineStages, &cfg.JobName, reg)
		if err != nil {
			return nil, err
		}

		// Add Source value to the static config target groups for unique identification
		// within scrape pool. Also, default target label to localhost if target is not
		// defined in promtail config.
		// Just to make sure prometheus target group sync works fine.
		for i, tg := range cfg.ServiceDiscoveryConfig.StaticConfigs {
			tg.Source = fmt.Sprintf("%d", i)
			if len(tg.Targets) == 0 {
				tg.Targets = []model.LabelSet{
					{model.AddressLabel: "localhost"},
				}
			}
		}

		// Add an additional api-level node filtering, so we only fetch pod metadata for
		// all the pods from the current node. Without this filtering we will have to
		// download metadata for all pods running on a cluster, which may be a long operation.
		for _, kube := range cfg.ServiceDiscoveryConfig.KubernetesSDConfigs {
			if kube.Role == kubernetes.RolePod {
				selector := fmt.Sprintf("%s=%s", kubernetesPodNodeField, hostname)
				kube.Selectors = []kubernetes.SelectorConfig{
					{Role: kubernetes.RolePod, Field: selector},
				}
			}
		}

		s := &targetSyncer{
			metrics:           metrics,
			log:               logger,
			positions:         positions,
			relabelConfig:     cfg.RelabelConfigs,
			targets:           map[string]*FileTarget{},
			droppedTargets:    []target.Target{},
			hostname:          hostname,
			entryHandler:      pipeline.Wrap(client),
			targetConfig:      targetConfig,
			fileEventWatchers: map[string]chan fsnotify.Event{},
			criClient:         criClient,
		}
		tm.syncers[cfg.JobName] = s
		configs[cfg.JobName] = cfg.ServiceDiscoveryConfig.Configs()
	}

	tm.wg.Add(3)
	go tm.run(ctx)
	go tm.watchTargetEvents(ctx)
	go tm.watchFsEvents(ctx)

	go util.LogError("running target manager", tm.manager.Run)

	return tm, tm.manager.ApplyConfig(configs)
}

func (tm *FileTargetManager) watchTargetEvents(ctx context.Context) {
	defer tm.wg.Done()

	for {
		select {
		case event := <-tm.targetEventHandler:
			switch event.eventType {
			case fileTargetEventWatchStart:
				if err := tm.watcher.Add(event.path); err != nil {
					level.Error(tm.log).Log("msg", "error adding directory to watcher", "error", err)
				}
			case fileTargetEventWatchStop:
				if err := tm.watcher.Remove(event.path); err != nil {
					level.Error(tm.log).Log("msg", " failed to remove directory from watcher", "error", err)
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (tm *FileTargetManager) watchFsEvents(ctx context.Context) {
	defer tm.wg.Done()

	for {
		select {
		case event := <-tm.watcher.Events:
			// we only care about Create events
			if event.Op == fsnotify.Create {
				level.Info(tm.log).Log("msg", "received file watcher event", "name", event.Name, "op", event.Op.String())
				for _, s := range tm.syncers {
					s.sendFileCreateEvent(event)
				}
			}
		case err := <-tm.watcher.Errors:
			level.Error(tm.log).Log("msg", "error from fswatch", "error", err)
		case <-ctx.Done():
			return
		}
	}
}

func (tm *FileTargetManager) run(ctx context.Context) {
	defer tm.wg.Done()

	for {
		select {
		case targetGroups := <-tm.manager.SyncCh():
			for jobName, groups := range targetGroups {
				tm.syncers[jobName].sync(groups, tm.targetEventHandler)
			}
		case <-ctx.Done():
			return
		}
	}
}

// Ready if there's at least one file target
func (tm *FileTargetManager) Ready() bool {
	for _, s := range tm.syncers {
		if s.ready() {
			return true
		}
	}
	return false
}

// Stop the TargetManager.
func (tm *FileTargetManager) Stop() {
	tm.quit()
	tm.wg.Wait()

	for _, s := range tm.syncers {
		s.stop()
	}
	util.LogError("closing watcher", tm.watcher.Close)
	close(tm.targetEventHandler)
}

// ActiveTargets returns the active targets currently being scraped.
func (tm *FileTargetManager) ActiveTargets() map[string][]target.Target {
	result := map[string][]target.Target{}
	for jobName, syncer := range tm.syncers {
		result[jobName] = append(result[jobName], syncer.ActiveTargets()...)
	}
	return result
}

// AllTargets returns all targets, active and dropped.
func (tm *FileTargetManager) AllTargets() map[string][]target.Target {
	result := map[string][]target.Target{}
	for jobName, syncer := range tm.syncers {
		result[jobName] = append(result[jobName], syncer.ActiveTargets()...)
		result[jobName] = append(result[jobName], syncer.DroppedTargets()...)
	}
	return result
}

// targetSyncer sync targets based on service discovery changes.
type targetSyncer struct {
	metrics      *Metrics
	log          log.Logger
	positions    positions.Positions
	entryHandler api.EntryHandler
	hostname     string

	fileEventWatchers map[string]chan fsnotify.Event

	droppedTargets []target.Target
	targets        map[string]*FileTarget
	mtx            sync.Mutex

	relabelConfig []*relabel.Config
	targetConfig  *Config

	criClient cri.Client
}

// sync synchronize target based on received target groups received by service discovery
func (s *targetSyncer) sync(groups []*targetgroup.Group, targetEventHandler chan fileTargetEvent) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	targets := map[string]struct{}{}
	dropped := []target.Target{}

	for _, group := range groups {
		for _, t := range group.Targets {
			level.Debug(s.log).Log("msg", "new target", "labels", t)

			discoveredLabels := group.Labels.Merge(t)
			var labelMap = make(map[string]string)
			for k, v := range discoveredLabels.Clone() {
				labelMap[string(k)] = string(v)
			}

			processedLabels := relabel.Process(labels.FromMap(labelMap), s.relabelConfig...)

			var labels = make(model.LabelSet)
			for k, v := range processedLabels.Map() {
				labels[model.LabelName(k)] = model.LabelValue(v)
			}

			// Drop empty targets (drop in relabeling).
			if processedLabels == nil {
				dropped = append(dropped, target.NewDroppedTarget("dropping target, no labels", discoveredLabels))
				level.Debug(s.log).Log("msg", "dropping target, no labels")
				s.metrics.failedTargets.WithLabelValues("empty_labels").Inc()
				continue
			}

			host, ok := labels[hostLabel]
			if ok && string(host) != s.hostname {
				dropped = append(dropped, target.NewDroppedTarget(fmt.Sprintf("ignoring target, wrong host (labels:%s hostname:%s)", labels.String(), s.hostname), discoveredLabels))
				level.Debug(s.log).Log("msg", "ignoring target, wrong host", "labels", labels.String(), "hostname", s.hostname)
				s.metrics.failedTargets.WithLabelValues("wrong_host").Inc()
				continue
			}

			// todo add custom path
			var (
				path        model.LabelValue
				containerId model.LabelValue
			)
			if fileBasedDiscovery, ok := labels["file_based_discovery"]; ok && string(fileBasedDiscovery) == "static_configs" {
				// is base file_based_discovery
				addressTarget, ok := labels["__address__"]
				if !ok {
					dropped = append(dropped, target.NewDroppedTarget("no path for target", discoveredLabels))
					level.Info(s.log).Log("msg", "no path for target", "labels", labels.String())
					s.metrics.failedTargets.WithLabelValues("no_path").Inc()
					continue
				}
				hostTargetSplit := strings.Split(string(addressTarget), ":")
				if len(hostTargetSplit) != 2 {
					continue
				}
				if hostTargetSplit[0] != "*" && hostTargetSplit[0] != s.hostname {
					continue
				}
				path = model.LabelValue(hostTargetSplit[1])

			} else { // case for kubernetes
				containerId, ok = labels[podContainerIdLabel]
				dplName, _ := labels[podControllerName]
				labels["containerId"] = containerId
				if ok {
					externalPath, internalPath, err := s.criClient.LogStoreDir(string(containerId))
					if err != nil {
						level.Error(s.log).Log("msg", "failed get docker inspect information")
					} else {
						path = model.LabelValue(s.customeContainerLogPath(externalPath, internalPath, string(dplName)))
					}
				} else {
					// standard workflow
					path, ok = labels[pathLabel]
					if !ok {
						dropped = append(dropped, target.NewDroppedTarget("no path for target", discoveredLabels))
						level.Info(s.log).Log("msg", "no path for target", "labels", labels.String())
						s.metrics.failedTargets.WithLabelValues("no_path").Inc()
						continue
					}
				}
			}

			pathExclude := labels[pathExcludeLabel]

			for k := range labels {
				if strings.HasPrefix(string(k), "__") {
					delete(labels, k)
				}
			}

			var key string
			if string(containerId) != "" { // containerId
				key = labels.String()
			} else {
				key = fmt.Sprintf("%s:%s", path, labels.String())
			}
			//if pathExclude != "" {
			//	key = fmt.Sprintf("%s:%s", key, pathExclude)
			//}
			targets[key] = struct{}{}
			if _, ok := s.targets[key]; ok { // target is existed,skip add
				dropped = append(dropped, target.NewDroppedTarget("ignoring target, already exists", discoveredLabels))
				level.Debug(s.log).Log("msg", "ignoring target, already exists", "labels", labels.String())
				s.metrics.failedTargets.WithLabelValues("exists").Inc()
				continue
			}

			level.Info(s.log).Log("msg", "Adding target", "key", key)

			wkey := string(path)
			watcher, ok := s.fileEventWatchers[wkey]
			if !ok {
				watcher = make(chan fsnotify.Event)
				s.fileEventWatchers[wkey] = watcher
			}
			t, err := s.newTarget(wkey, string(pathExclude), labels, discoveredLabels, watcher, targetEventHandler)
			if err != nil {
				dropped = append(dropped, target.NewDroppedTarget(fmt.Sprintf("Failed to create target: %s", err.Error()), discoveredLabels))
				level.Error(s.log).Log("msg", "Failed to create target", "key", key, "error", err)
				s.metrics.failedTargets.WithLabelValues("error").Inc()
				continue
			}

			s.metrics.targetsActive.Add(1.)
			s.targets[key] = t
		}
	}

	for key, target := range s.targets {
		if _, ok := targets[key]; !ok {
			level.Info(s.log).Log("msg", "Removing target", "key", key)
			target.Stop()
			s.metrics.targetsActive.Add(-1.)
			delete(s.targets, key)

			// close related file event watcher
			k := target.path
			if _, ok := s.fileEventWatchers[k]; ok {
				close(s.fileEventWatchers[k])
				delete(s.fileEventWatchers, k)
			} else {
				level.Warn(s.log).Log("msg", "failed to remove file event watcher", "path", k)
			}
		}
	}
	s.droppedTargets = dropped
}

// sendFileCreateEvent sends file creation events to only the targets with matched path.
func (s *targetSyncer) sendFileCreateEvent(event fsnotify.Event) {
	// Lock the mutex because other threads are manipulating s.fileEventWatchers which can lead to a deadlock
	// where we send events to channels where nobody is listening anymore
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for path, watcher := range s.fileEventWatchers {
		matched, err := doublestar.Match(path, event.Name)
		if err != nil {
			level.Error(s.log).Log("msg", "failed to match file", "error", err, "filename", event.Name)
			continue
		}
		if !matched {
			level.Debug(s.log).Log("msg", "new file does not match glob", "filename", event.Name)
			continue
		}
		watcher <- event
	}
}

func (s *targetSyncer) newTarget(path, pathExclude string, labels model.LabelSet, discoveredLabels model.LabelSet, fileEventWatcher chan fsnotify.Event, targetEventHandler chan fileTargetEvent) (*FileTarget, error) {
	return NewFileTarget(s.metrics, s.log, s.entryHandler, s.positions, path, pathExclude, labels, discoveredLabels, s.targetConfig, fileEventWatcher, targetEventHandler)
}

func (s *targetSyncer) DroppedTargets() []target.Target {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return append([]target.Target(nil), s.droppedTargets...)
}

func (s *targetSyncer) ActiveTargets() []target.Target {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	actives := []target.Target{}
	for _, t := range s.targets {
		actives = append(actives, t)
	}
	return actives
}

func (s *targetSyncer) ready() bool {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for _, target := range s.targets {
		if target.Ready() {
			return true
		}
	}
	return false
}

func (s *targetSyncer) stop() {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for key, target := range s.targets {
		level.Info(s.log).Log("msg", "Removing target", "key", key)
		target.Stop()
		delete(s.targets, key)
	}

	for key, watcher := range s.fileEventWatchers {
		close(watcher)
		delete(s.fileEventWatchers, key)
	}
	s.entryHandler.Stop()
}

func hostname() (string, error) {
	hostname := os.Getenv("HOSTNAME")
	if hostname != "" {
		return hostname, nil
	}

	return os.Hostname()
}

// customeContainerLogPath get some custom log in pod by combine some hard code with some path get from docker inspect
func (s *targetSyncer) customeContainerLogPath(externalPath, internalPath, replicasName string) string {

	var applicationLogs = []string{
		"/appJson/jsonApp.*.log",
		"/app_json_logs/application.*.log",
		"/access.*.log",
		"/dubboAccess.*.log",
		"/sql.*.log",
		"/gc.log",
		"/memory.log",
		"/application.log",
		"/app.log",
		"/DubboStack.*.log",
		"/providence.log",
	}

	var (
        pathString = "{"
    )

	if externalPath != "" {
		logDir, ok := replicasName2LogName(replicasName)
		if !ok {
			level.Warn(s.log).Log("msg", "targetSync reject target, for config hostPath, but dpl name invalidate", "replicaset", replicasName)
			//  容器名称非法不符合 dmo-lego-xxx-deployment格式
			return ""
		}

        prefix := "/rootfs/" + externalPath + "/" + logDir
		for _, logDir := range applicationLogs {
			pathString += path.Join(prefix, logDir) + ","
		}

		return pathString + "}"
	}

	if internalPath != "" {
		for _, logDir := range applicationLogs {
			pathString += path.Join(internalPath, "/root/logs/*", logDir) + ","
		}
	}

	return pathString + "}"
}

// logDirname get deployment from controllerName
// in common case controllerName is represent the name of replicaset
func replicasName2LogName(repliacsName string) (string, bool) {
	/* if strings.HasPrefix(repliacsName, "dmo-lego-") == false {
		return "", false
	} */
	nSplit := strings.Split(repliacsName, "-")
	if len(nSplit) > 3 {
		return strings.Join(nSplit[1:len(nSplit)-2], "-"), true
	}
	return "", false
}
