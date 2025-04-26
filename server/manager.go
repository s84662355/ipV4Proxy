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
	"sync/atomic"

	"github.com/orcaman/concurrent-map/v2"
	"go.uber.org/zap" // 高性能日志库
	"proxy_server/protobuf"

	"google.golang.org/grpc"
	"proxy_server/log"
	"proxy_server/utils/taskConsumerManager"
)

const acceptAmount = 16

// 使用 sync.OnceValue 确保 manager 只被初始化一次（线程安全）
var newManager = sync.OnceValue(func() *manager {
	m := &manager{
		tcm:            taskConsumerManager.New(), // 任务消费者管理器
		ipConnCountMap: cmap.New[*IpConnCountMapData](),
		userCtxMap:     cmap.New[*connContext](),
	}
	m.isRun.Store(true)
	m.bytePool = sync.Pool{
		// 当池为空时，使用 New 函数创建新对象
		New: func() interface{} {
			return make([]byte, 1024)
		},
	}

	return m
})

// manager 结构体管理整个代理服务的核心组件
type manager struct {
	protobuf.UnimplementedAuthServer
	tcm            *taskConsumerManager.Manager // 任务调度管理器
	tcpListener    net.Listener
	grpcServer     *grpc.Server
	grpcListener   net.Listener
	isRun          atomic.Bool
	bytePool       sync.Pool
	ipConnCountMap cmap.ConcurrentMap[string, *IpConnCountMapData]
	userCtxMap     cmap.ConcurrentMap[string, *connContext]
}

// Start 启动代理服务的各个组件
func (m *manager) Start() error {
	m.initTcpListener()

	m.tcm.AddTask(acceptAmount, m.tcpAccept)
	m.tcm.AddTask(1, m.runGrpcServer)

	return nil
}

// Stop 停止所有服务组件
func (m *manager) Stop() {
	m.isRun.Store(false)
	m.tcpListener.Close()
	m.tcm.Stop() // 停止任务消费者管理器，会触发所有任务的优雅关闭
}

func (m *manager) AddIpConnCount(ip string) bool {
	ok := false
	m.ipConnCountMap.Upsert(
		ip, nil,
		func(exist bool, valueInMap *IpConnCountMapData, newValue *IpConnCountMapData) *IpConnCountMapData {
			if exist {
				conf := config.GetConf()
				if conf.OneIpMaxConn != 0 && valueInMap.count.Load() >= conf.OneIpMaxConn {
					return valueInMap
				}
				valueInMap.count.Add(1)
				ok = true
				return valueInMap
			}

			valueInMap := &IpConnCountMapData{}
			valueInMap.count.Add(1)
			ok = true
			return valueInMap
		})

	return ok
}

func (m *manager) ReduceIpConnCount(ip string) {
	m.ipConnCountMap.RemoveCb(
		ip,
		func(key string, valueInMap *IpConnCountMapData, exists bool) bool {
			if exist {
				if valueInMap.count.Add(-1) == 0 {
					return true
				}
			}
			return false
		})
}

func (m *Manager) addUserConnection(k string) *connContext {
	return m.userCtxMap.Upsert(k, nil, func(exist bool, valueInMap *connContext, newValue *connContext) *connContext {
		if exist {
			valueInMap.c++
			return valueInMap
		}
		ctx, cancel := context.WithCancel(m.tcm.Context())
		return &connContext{
			ctx:    ctx,
			cancel: cancel,
			c:      1,
			// a:      io.NewLimitedReaderAction(ctx,conf.Conf.LimitedReader.ReadRate, conf.Conf.LimitedReader.ReadBurst),
		}
	})
}

func (m *Manager) deleteUserConnection(k string, ctx *connContext) {
	m.userCtxMap.RemoveCb(k, func(key string, valueInMap *connContext, exists bool) bool {
		if exists {
			if valueInMap != ctx {
				return false
			}
			valueInMap.c--
			if valueInMap.c == 0 {
				valueInMap.cancel()
				return true
			}
			return false
		}

		return true
	})
}

func (m *Manager) CloseUserConnections(k string) error {
	v, exist := m.userCtxMap.Pop(k)

	if exist && v != nil {
		v.cancel()
		return nil
	}

	return fmt.Errorf("CloseUserConnections userCtxMap %s 不存在", k)
}
