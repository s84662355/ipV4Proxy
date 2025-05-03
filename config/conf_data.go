package config

type confData struct {
	TcpListenerAddress  []string
	GrpcListenerAddress string
	LogDir              string
	LocalIp             string
	ProcessName         string
	Redis               *redis_config
	Rabbitmq            *rabbitmq_config
	Nacos               *nacos_config
}
