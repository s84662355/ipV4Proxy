package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"proxy_server/log"
	"proxy_server/server"
)

var configPath string

func init() {
	flag.StringVar(&configPath, "config_path", "", "配置文件路径")
	flag.Parse()

	fmt.Println(configPath)

	config.Load(configPath)
	log.Init(filepath.Join(config.GetConf().LogDir, config.ServiceName, mark))
	go gohttp()
}

// main 程序主入口
func main() {
	// 1. 启动服务器
	err := server.Start()
	if err != nil {
		// 启动失败记录致命错误并退出
		log.Fatal("服务器启动失败",
			zap.String("error", err.Error()),
			zap.String("version", version))
		return
	}

	// 确保程序退出时执行清理工作
	defer func() {
		server.Stop() // 停止服务器
		log.Info("程序正常关闭")
	}()

	// 2. 设置信号监听通道
	signalChan := make(chan os.Signal, 1) // 缓冲大小为1的信号通道

	// 注册要监听的信号：
	// - SIGINT (Ctrl+C中断)
	// - SIGTERM (终止信号)
	// - Kill信号 (强制终止)
	signal.Notify(signalChan,
		syscall.SIGINT,
		syscall.SIGTERM,
		os.Kill)

	// 初始时间间隔为 30 秒
	interval := 30 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		ticker.Reset(interval)
		select {
		case <-ticker.C:
			config.Load(configPath)
			// 3. 主线程阻塞等待终止信号
		case <-signalChan: // 当接收到上述任意信号时继续执行
			return
		}

	}
}
