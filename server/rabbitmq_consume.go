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

	"proxy_server/config"
	"proxy_server/protobuf"

	"github.com/streadway/amqp"
	"google.golang.org/grpc"
	"proxy_server/log"
	"proxy_server/utils/taskConsumerManager"
)

func (m *manager) runRabbitmqConsume(ctx context.Context) {
	amqpUrl := fmt.Sprintf("amqp://%s:%s@%s:%d/%s",
		config.GetConf().Rabbitmq.Host,
		config.GetConf().Rabbitmq.Port,
		config.GetConf().Rabbitmq.User,
		config.GetConf().Rabbitmq.Password,
		config.GetConf().Rabbitmq.VirtualHost,
	)

	conn, err := amqp.Dial(amqpUrl)
	if err != nil {
		log.Error("[rabbitmq_consume] rabbitmq  amqp.Dial 错误", zap.Error(err))
		timeOutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		<-timeOutCtx.Done()
		return
	}
	defer conn.Close()

	closeChan := conn.NotifyClose(make(chan *amqp.Error, 1))

	tcm := taskConsumerManager.New()
	defer tcm.Stop()

	tcm.AddTask(1, func(ctx context.Context) {
		m.runRabbitmqUserInfoQueueConsume(conn, ctx)
	})

	select {
	case <-closeChan:
	case <-ctx.Done():
	}

	timeOutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	<-timeOutCtx.Done()

	conn.Close()
}

func (m *manager) runRabbitmqUserInfoQueueConsume(conn *amqp.Connection, ctx context.Context) {
	ch, err := conn.Channel()
	if err != nil {
		log.Error("[rabbitmq_consume] rabbitmq conn.Channel 错误", zap.Error(err))
		timeOutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		<-timeOutCtx.Done()
		return
	}
	defer ch.Close()

	// 声明一个队列
	q, err := ch.QueueDeclare(
		"hello", // 队列名称
		true,    // 是否持久化
		false,   // 是否自动删除
		false,   // 是否排他
		false,   // 是否等待服务器响应
		nil,     // 额外参数
	)
	if err != nil {
		log.Error("[rabbitmq_consume] rabbitmq ch.QueueDeclare 错误", zap.Error(err))
		timeOutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		<-timeOutCtx.Done()
		return
	}

	tcm := taskConsumerManager.New()
	defer tcm.Stop()

	tcm.AddTask(1, func(ctx context.Context) {
		// 从队列中消费消息
		msgs, err := ch.Consume(
			q.Name, // 队列名称
			"",     // 消费者名称
			true,   // 是否自动确认
			false,  // 是否排他
			false,  // 是否为本地队列
			false,  // 是否等待服务器响应
			nil,    // 额外参数
		)
		if err != nil {
			log.Error("[rabbitmq_consume] rabbitmq ch.Consume 错误", zap.Error(err))
			timeOutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			<-timeOutCtx.Done()
			return
		}

		for d := range msgs {
			log.Printf("Received a message: %s", d.Body)
		}

		timeOutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		<-timeOutCtx.Done()
	})
	closeChan := ch.NotifyClose(make(chan *amqp.Error, 1))

	select {
	case <-closeChan:
	case <-ctx.Done():
	}

	timeOutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	<-timeOutCtx.Done()
	ch.Close()
}
