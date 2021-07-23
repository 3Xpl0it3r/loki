package filesystem

import (
	"context"
	"github.com/grafana/loki/clients/pkg/promtail/api"
)

// 文件操作句柄接口
type Handler interface {
	// 推出关闭，清理资源
	close()
	//  开始运行handler，每个handler都打开一个文件，写文件
	run(ctx context.Context)
	// 从client接收资源
	Chan() chan<- api.Entry
}
