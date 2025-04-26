package server

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	HTTP_LOG_PRE             = "http_proxy:"
	SOCKS5_LOG_PRE           = "socks5_proxy:"
	LOG_PRE                  = "Title:"
	LOG_MESSAGE              = "msg:"
	PROTOCEL_TPC             = "tcp"
	PROTOCEL_UDP             = "udp"
	PROTOCEL_SOCKS5          = "socks5"
	PROTOCEL_HTTP            = "http"
	PROTOCEL_SPS             = "sps"
	ZERO_INT                 = 0
	REQ_METHOD_CONNECT       = "CONNECT"
	CONN_WRITE_TIME    int64 = 180
	CONN_READ_TIME     int64 = 180
	IN_STR                   = "in"
	OUT_STR                  = "out"
	EOF_STR                  = "EOF"
	DISCONNECT_CHANNEL       = "disconnect"
)

type connContext struct {
	ctx    context.Context
	cancel context.CancelFunc
	// a      *io.LimitedReaderAction
	c uint64
}

type IpConnCountMapData struct {
	count atomic.Int64
}

func newConn(conn net.Conn, writetimeout, readtimeout int64) *Conn {
	return &Conn{
		conn:         conn,
		readtimeout:  time.Duration(readtimeout) * time.Second,
		writetimeout: time.Duration(writetimeout) * time.Second,
	}
}

type Conn struct {
	conn         net.Conn
	readtimeout  time.Duration
	writetimeout time.Duration
}

func (c *Conn) Write(p []byte) (n int, err error) {
	// 设置写超时
	c.conn.SetWriteDeadline(time.Now().Add(c.writetimeout))
	return c.conn.Write(p)
}

func (c *Conn) Read(p []byte) (n int, err error) {
	c.conn.SetReadDeadline(time.Now().Add(c.readtimeout))
	return c.conn.Read(p)
}

type LimitedReaderAction struct {
	readRate         int // 每次读取字节数
	readBurst        int // 每秒max读取字节数
	wMutex           sync.RWMutex
	nowTimeUnix      int64
	residReadueBurst int
}

func NewLimitedReaderAction(readRate, readBurst int) *LimitedReaderAction {
	if readRate <= 0 {
		readRate = 1024 * 30
	}
	if readBurst <= 0 {
		readBurst = 1024 * 100000
	}

	return &LimitedReaderAction{
		readRate:         readRate,
		readBurst:        readBurst,
		residReadueBurst: readBurst,
		nowTimeUnix:      time.Now().Unix(),
	}
}

func (l *LimitedReaderAction) ReadBurst() int {
	return l.readBurst
}

func (l *LimitedReaderAction) UpdateParameter(readRate, readBurst int) {
	l.wMutex.Lock()
	defer l.wMutex.Unlock()

	if readRate <= 0 {
		readRate = 1024 * 30
	}
	if readBurst <= 0 {
		readBurst = 1024 * 100000
	}

	l.readRate = readRate
	l.readBurst = readBurst
	l.residReadueBurst = readBurst
}

func (l *LimitedReaderAction) GetNowTimeUnix() int64 {
	return l.nowTimeUnix
}

func (l *LimitedReaderAction) GetReadSize(dataLen int) int {
	l.wMutex.Lock()
	defer l.wMutex.Unlock()
	if l.nowTimeUnix != time.Now().Unix() {
		l.nowTimeUnix = time.Now().Unix()
		l.residReadueBurst = l.readBurst
	}

	if l.residReadueBurst == 0 {
		return 0
	}

	if l.readRate >= l.residReadueBurst {
		temp := l.residReadueBurst
		l.residReadueBurst = 0

		if dataLen > temp {
			return temp
		}
		return dataLen
	}

	if dataLen > l.readRate {
		l.residReadueBurst -= l.readRate
		return l.readRate
	}
	l.residReadueBurst -= dataLen

	return dataLen
}

// 流量限制
type LimitedReader struct {
	r      Reader // underlying reader
	buf    []byte
	llen   int
	action *LimitedReaderAction
}

func NewLimitedReader(r io.Reader, action *LimitedReaderAction) *LimitedReader {
	return &LimitedReader{
		r:      r,
		llen:   0,
		action: action,
		buf:    make([]byte, 4*1024),
	}
}

func (l *LimitedReader) Read(p []byte) (n int, err error) {
	if l.llen == 0 {
		l.buf = make([]byte, 4*1024)
		l.llen, err = l.r.Read(l.buf)
		if err != nil {
			return 0, err
		}
	}

	if l.llen <= 0 {
		return 0, EOF
	}
	size := 0
	// for true {
	size = l.action.GetReadSize(l.llen)

	/*
		if size > 0 {
			break
		}
	*/
	if size <= 0 {
		time.Sleep(33 * time.Millisecond)
		size = 1
	}

	//break
	//}

	if len(p) > size {
		p = p[:size]
	}

	copy(p, l.buf[:len(p)])

	l.llen -= len(p)

	l.buf = l.buf[len(p):]

	return len(p), nil
}

/*
func (l *LimitedReader) Read(p []byte) (n int, err error) {

	// 1、如果限制字节数小于0则返回	EOF
	if l.n <= 0 {
		return 0, EOF
	}
	// 2、如果 读取的缓冲器大小 大于 限制字节数，则将p截断
	if len(p) > l.n {
		p = p[0:l.n]
	}
	// 3、开始读取 n个字节进入 p
	n, err = l.r.Read(p)
	// 4、剩余字节数减少n
	l.n -= n
	return

}


*/
