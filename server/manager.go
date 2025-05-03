package server

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/orcaman/concurrent-map/v2"
	"github.com/spf13/viper"

	"google.golang.org/grpc"

	"proxy_server/protobuf"
	"proxy_server/utils/Queue"
	"proxy_server/utils/rabbitMQ"
	"proxy_server/utils/taskConsumerManager"
)

const AcceptAmount = 8

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
			return make([]byte, 2*1024)
		},
	}

	return m
})

// manager 结构体管理整个代理服务的核心组件
type manager struct {
	protobuf.UnimplementedAuthServer
	tcm                            *taskConsumerManager.Manager // 任务调度管理器
	tcpListener                    map[string]net.Listener
	grpcServer                     *grpc.Server
	grpcListener                   net.Listener
	isRun                          atomic.Bool
	bytePool                       sync.Pool
	ipConnCountMap                 cmap.ConcurrentMap[string, *IpConnCountMapData]
	userCtxMap                     cmap.ConcurrentMap[string, *connContext]
	nacosConfig                    *NacosConfig
	nacosConfigMu                  sync.RWMutex
	viperClient                    *viper.Viper
	blackMap                       atomic.Pointer[map[string]struct{}]
	rabbitmqSendQueueSlices        []Queue.Queue[*rabbitMQ.RabbitMqData]
	rabbitmqSendQueueSlicesCounter atomic.Uint64
	rabbitmqSendQueueDone          chan struct{}
	nacosRespChan                  <-chan bool
}

// Start 启动代理服务的各个组件
func (m *manager) Start() error {
	m.nacosConfig = &NacosConfig{}
	m.initNacosConf()
	m.initTcpListener()
	m.initRabbitmqSendQueueSlices()

	m.tcm.AddTask(AcceptAmount, m.tcpAccept)
	// m.tcm.AddTask(1, m.runGrpcServer)
	m.tcm.AddTask(1, m.runNacosConfServer)
	m.tcm.AddTask(1, m.runRabbitmqConsume)

	return nil
}

// Stop 停止所有服务组件
func (m *manager) Stop() {
	m.isRun.Store(false)
	for _, v := range m.tcpListener {
		v.Close()
	}
	m.tcm.Stop() // 停止任务消费者管理器，会触发所有任务的优雅关闭
	m.closeRabbitmqSendQueueSlices()
}

func (m *manager) AddIpConnCount(ip string) (ok bool, ipCount int64) {
	m.ipConnCountMap.Upsert(
		ip, nil,
		func(exist bool, valueInMap *IpConnCountMapData, newValue *IpConnCountMapData) *IpConnCountMapData {
			if exist {
				conf := m.getNacosConf()
				if conf.OneIpMaxConn != 0 && valueInMap.count.Load() >= int64(conf.OneIpMaxConn) {
					ipCount = valueInMap.count.Load()
					return valueInMap
				}
				ipCount = valueInMap.count.Add(1)
				ok = true
				return valueInMap
			}

			valueInMap = &IpConnCountMapData{}
			ipCount = valueInMap.count.Add(1)
			ok = true
			return valueInMap
		})

	return
}

func (m *manager) ReduceIpConnCount(ip string) {
	m.ipConnCountMap.RemoveCb(
		ip,
		func(key string, valueInMap *IpConnCountMapData, exists bool) bool {
			if exists {
				if valueInMap.count.Add(-1) == 0 {
					return true
				}
			}
			return false
		})
}

func (m *manager) addUserConnection(k string) *connContext {
	return m.userCtxMap.Upsert(k, nil, func(exist bool, valueInMap *connContext, newValue *connContext) *connContext {
		if exist {
			valueInMap.c++
			return valueInMap
		}
		conf := m.getNacosConf()
		ctx, cancel := context.WithCancel(m.tcm.Context())
		return &connContext{
			ctx:    ctx,
			cancel: cancel,
			c:      1,
			a:      NewLimitedReaderAction(conf.LimitedReader.ReadRate, conf.LimitedReader.ReadBurst),
		}
	})
}

func (m *manager) deleteUserConnection(k string, ctx *connContext) {
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

func (m *manager) CloseUserConnections(k string) error {
	v, exist := m.userCtxMap.Pop(k)

	if exist && v != nil {
		v.cancel()
		return nil
	}

	return fmt.Errorf("CloseUserConnections userCtxMap %s 不存在", k)
}

func (m *manager) SetBlackMap(bm map[string]struct{}) {
	m.blackMap.Store(&bm)
}

func (m *manager) GetBlackMap() *map[string]struct{} {
	return m.blackMap.Load()
}

func (m *manager) IsInBlacklist(d string) (string, bool) {
	bm := m.blackMap.Load()
	if bm == nil {
		return "", false
	}
	blackMap := *bm

	_, ok := blackMap[d]
	if ok {
		return d, true
	}

	for vv := range blackMap {
		if strings.Contains(d, vv) {
			return vv, true
		}
	}

	return "", false
}
