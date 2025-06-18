package config

type Config struct {
	Port   string
	DBPath string
	APIKey string
}

func Load() Config {
	return Config{
		Port:   "8080",
		DBPath: "./sms_service.db",
		APIKey: "qwerty123",
	}
}
