package config

import (
	"os"
	"strings"
)

type Config struct {
	Namespaces []string
}

func GetConfigOrDie() *Config {
	cfg := Config{}

	ns, ok := os.LookupEnv("WATCH_NAMESPACE")
	if ok {
		cfg.Namespaces = strings.Split(ns, ",")
	}

	return &cfg
}
