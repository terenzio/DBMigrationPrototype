package config

import "os"

type DBConfig struct {
	Host     string
	Port     string
	Service  string
	User     string
	Password string
}

func LoadDBConfig() DBConfig {
	return DBConfig{
		Host:     getEnv("ORACLE_HOST", "oracle-xe"),
		Port:     getEnv("ORACLE_PORT", "1521"),
		Service:  getEnv("ORACLE_SERVICE", "XEPDB1"),
		User:     getEnv("ORACLE_USER", "system"),
		Password: getEnv("ORACLE_PASSWORD", "oracle"),
	}
}

func (c DBConfig) DSN() string {
	return c.User + "/" + c.Password + "@" + c.Host + ":" + c.Port + "/" + c.Service
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
