package filesystem

import (
	"errors"
	"github.com/grafana/loki/clients/pkg/promtail/api"
	"path"
	"strings"
	"sync"
)

var (
	_pool = &metaPool{p: &sync.Pool{New: func() interface{} {
		return &metadata{}
	}}}
	Get = _pool.Get
	Put = _pool.Put
)

type metaPool struct {
	p *sync.Pool
}

func (p metaPool) Put(m *metadata) {
	p.p.Put(m)
}
func (p metaPool) Get() *metadata {
	return p.p.Get().(*metadata)
}

// metadata 写文件所需要的元数据，group/service/app 拼接目录
type metadata struct {
	// namespace kubernetes
	namespace string
	// name of controller , common case in appear in kubernetes environment
	// in our case controllerName always represent replicaset name
	controllerName string
	// instance represent host that pod located
	instance string
	// fileName represent the log name that need to be gather by promtail
	// fileName is differ from originFilename
	// fileName is not contains path, but originFilename is an absolute filename that combine filename and an absolute path
	fileName       string
	originFilename string
	isKubernetes   bool
}

// BasePath is return the base path
func (m metadata) BasePathFmt(root string) string {
	var base string
	if m.isKubernetes {
		base = root + "/" + m.namespace + "/" + DeploymentName(m.controllerName) + "/" + "%s" + "/" + m.instance
	}else {
		base = root + "/" + m.namespace + "/" + DeploymentName(m.controllerName) + "/"  + m.instance
	}
	if m.isKubernetes == false{
		tPath := path.Dir(m.originFilename)
		if tPath != "."{
			base = base + "/" + tPath
		}
		return base
	}
	return  base
}

func (m metadata) HandlerId() string {
	return m.namespace + "." + m.controllerName + "." + m.instance + "." + m.originFilename
}



//根据label生成元数据
func parseEntry(entry *api.Entry, md *metadata) error {
	md.isKubernetes = true
	if md == nil {
		return errors.New("nil")
	}

	if value, ok := entry.Labels[NamespaceLabel]; ok {
		md.namespace = string(value)
	} else {
		md.isKubernetes = false
		md.namespace = defaultNamespace
	}
	if value, ok := entry.Labels[ControllerNameLabel]; ok {
		md.controllerName = string(value)
	} else {
		md.isKubernetes = false
		md.controllerName = defaultControllerName
	}
	if value, ok := entry.Labels[InstanceLabel]; ok {
		md.instance = string(value)
	} else {
		md.instance = defaultInstanceName
	}
	if value, ok := entry.Labels[FileNameLabel]; ok {
		md.originFilename = string(value)
		md.fileName = getFileBaseName(string(value))
	} else {
		return errors.New("path label must be existed")
	}
	return nil
}

func getFileBaseName(fileName string) string {
	return path.Base(fileName)
}

// DeploymentName get deployment from controllerName
// in common case controllerName is represent the name of replicaset
func DeploymentName(controllerName string) string {
	nSplit := strings.Split(controllerName, "-")
	if len(nSplit) >= 2 {
		return strings.Join(nSplit[:len(nSplit)-1], "-")
	}
	return controllerName
}
