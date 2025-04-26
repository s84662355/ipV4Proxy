package server

import (
	"io"
	"net"

	"go.uber.org/zap"

	"proxy_server/log"
	"proxy_server/util/socks5"
)

func (m *manager) socksTcpConn(ctx context.Context, conn net.Conn) {
	// 读取账号密码
	var user, pwd string
	user, pwd, err = socks5.GetUserPassword(conn)
	if err != nil {
		log.Error("[socks_proxy_handler] 读取账号密码错误", zap.Error(err))
		if _, err = conn.Write([]byte{socks5.UserAuthVersion, socks5.AuthFailure}); err != nil {
			return
		}
		return
	}

	proxyServerConn := conn.LocalAddr().(*net.TCPAddr)
	proxyServerIpStr := proxyServerConn.IP.String()
	proxyServerIpByte := proxyServerConn.IP.To4()

	_, err = m.proxyAuth.Valid(ctx, user, pwd, proxyServerIpStr)
	if err != nil {
		log.Error("[socks_proxy_handler] 鉴权失败", zap.Error(err), zap.Any("user", user), zap.Any("pwd", pwd), zap.Any("ip", proxyServerIpStr))
		if _, err = conn.Write([]byte{socks5.UserAuthVersion, socks5.AuthFailure}); err != nil {
			return
		}
		return
	}

	if m.AddIpConnCount(proxyServerIpStr) {
		defer m.ReduceIpConnCount(proxyServerIpStr)
	}

	///认证成功，返回消息给客户端
	if _, err = conn.Write([]byte{socks5.UserAuthVersion, socks5.AuthSuccess}); err != nil {
		return
	}

	var destAddr *socks5.AddrSpec
	destAddr, err = socks5.ReadDestAddr(conn)
	if err != nil {
		if err == socks5.UnrecognizedAddrType {
			if err = socks5.SendReply(conn, socks5.AddrTypeNotSupported, nil); err != nil {
				return
			}
		}
		return
	}

	var target net.Conn
	target, err = DialContext("tcp", destAddr.Address(), time.Second*10, proxyServerIpByte, 0)
	if err != nil {
		log.Error("[socks_proxy_handler] DialContext 创建目标连接失败", zap.Error(err))
		msg := err.Error()
		resp := socks5.HostUnreachable
		if strings.Contains(msg, "refused") {
			resp = socks5.ConnectionRefused
		} else if strings.Contains(msg, "network is unreachable") {
			resp = socks5.NetworkUnreachable
		}
		if err = socks5.SendReply(conn, resp, nil); err != nil {
			return
		}
		return
	}
	defer target.Close()

	clientAddr := conn.RemoteAddr().String()
	log.Info("[socks_proxy_handler] 创建目标连接成功 ",
		zap.Any("username", user),
		zap.Any("s5_proxy_ip", proxyServerIpStr),
		zap.Any("clientAddr", clientAddr),
		zap.Any("destAddr", destAddr.Address()),
	)

	// 告诉客户端连接目标服务器成功
	local := target.LocalAddr().(*net.TCPAddr)
	bind := socks5.AddrSpec{IP: local.IP, Port: local.Port}
	if err = socks5.SendReply(conn, socks5.SuccessReply, &bind); err != nil {
		log.Error("[socks_proxy_handler] 应答socks5.SuccessReply失败", zap.Error(err))
		return
	}

	key := fmt.Sprintf("%s:%s", user, proxyServerIpStr)
	connCtx := m.addUserConnection(key)
	// action := connCtx.a
	defer m.deleteUserConnection(key, connCtx)

	netConn := newConn(conn, CONN_WRITE_TIME, CONN_READ_TIME)
	netTarget := newConn(target, CONN_WRITE_TIME, CONN_READ_TIME)

	///////////////////////
	////黑名单逻辑
	//		level.Info(m.logger).Log(LOG_PRE, fmt.Sprintf("%s start to transport!", s5LogPre), LOG_MESSAGE, fmt.Sprintf("s5_proxy_ip:[%s] - clientAddr:[%s] - username:[%s] - target_host:[%s]", proxyServerIpStr, clientAddr, user, serverName))

	//////////////////////

	///go rabbitmq.ReportAccessLogToInfluxDB(user, serverName, conn.LocalAddr().String()) // 访问数据插入时序数据库

	errCh := make(chan error, 2)
	defer close(errCh)

	wg := sync.WaitGroup{}
	defer wg.Wait()
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := io.CopyBuffer(netTarget, io.NewLimitedReader(netConn, action), make([]byte, 2*1024))
		errCh <- err
	}()

	go func() {
		defer wg.Done()
		_, err := io.CopyBuffer(netConn, io.NewLimitedReader(netTarget, action), make([]byte, 2*1024))
		errCh <- err
	}()

	select {
	case err, _ := <-errCh:
		if err != nil {
			log.Error("[socks_proxy_handler] conn close!",
				zap.Error(err),
				zap.Any("username", user),
				zap.Any("clientAddr", clientAddr),
				zap.Any("target_host", serverName),
			)
		}
	case <-ctx.Done():

	case <-connCtx.ctx.Done():

	}

	target.Close()
	conn.Close()
}
