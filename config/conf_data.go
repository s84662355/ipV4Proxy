package config

type confData struct {
	TcpListenerAddress  string
	GrpcListenerAddress string
	OneIpMaxConn        int64
	LogDir              string
	Redis               *redis_config
	Rabbitmq            *rabbitmq_config
}
