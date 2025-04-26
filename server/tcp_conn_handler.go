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

func (m *manager) handlerTcpConn(ctx context.Context, conn net.Conn) {
	buffer := m.bytePool.Get().([]byte)

	n, err = conn.Read(buffer)
	if err != nil {
		return
	}

	// socks5
	if n <= 8 {
		m.bytePool.Put(buffer)
		return m.socksTcpConn(ctx, conn)
	} else { // http
		var req *httpProxy.Request
		req, err = httpProxy.NewRequestOnByte(&conn, buffer, n)
		if err != nil {
			level.Error(m.logger).Log("decoder error , form %s, ERR:%s", err, conn.RemoteAddr())
			io.Close(conn)
			return
		}
		return m.runHttp(conn, req)
	}
}

//     // 使用 bytes.Buffer 将 []byte 转换为 io.Reader
//     buffer := bytes.NewBuffer(data)

//     // 使用 bufio.NewReader 创建 *bufio.Reader
//     reader := bufio.NewReader(buffer)

// ReadRequest(reader) (*Request, error)
