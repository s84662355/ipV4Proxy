package server

import (
	"context"
	"encoding/json"
	"flag"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/lysShub/divert-go"             // Windows 网络数据包捕获库
	"go.uber.org/zap"                          // 高性能日志库
	"gvisor.dev/gvisor/pkg/tcpip/link/channel" // gVisor 的网络栈实现

	"proxy_server/config"
	"proxy_server/log"
	"proxy_server/utils/taskConsumerManager"
)

func (m *manager) initTcpListener() {
	conf := config.GetConf()

	listener, err := net.Listen("tcp", conf.TcpListenerAddress)
	if err != nil {
		log.Panic("[tcp_server] 初始化tcp监听服务失败", zap.Error(err))
	}

	m.tcpListener = listener
}

func (m *manager) tcpAccept(ctx context.Context) {
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	for m.isRun.Load() {
		// 接受客户端的连接
		conn, err := m.tcpListener.Accept()
		if err != nil {
			// 若接受连接出错，记录错误信息并继续等待下一个连接
			log.Panic("[tcp_server]  tcp监听服务Accept", zap.Error(err))
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer conn.Close()
			m.handlerTcpConn(ctx, conn)
		}()
	}
}
