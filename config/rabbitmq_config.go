package config

type rabbitmq_config struct {
	Host              string
	Port              int
	User              string
	Password          string
	VirtualHost       string
	BlacklistExchange string
}
