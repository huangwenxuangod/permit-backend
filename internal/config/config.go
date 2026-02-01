package config

import (
	"os"
	"strconv"
)

type Config struct {
	Env        string
	Port       int
	AssetsDir  string
	UploadsDir string
	JWTSecret  string
	LogJSON    bool
	AlgoURL    string
}

func Default() Config {
	return Config{
		Env:        "dev",
		Port:       5000,
		AssetsDir:  "./assets",
		UploadsDir: "./uploads",
		JWTSecret:  "",
		LogJSON:    true,
		AlgoURL:    "http://127.0.0.1:8080",
	}
}

func EnvDefaults() Config {
	return fromEnv(Default())
}

func fromEnv(c Config) Config {
	if v := os.Getenv("PERMIT_ENV"); v != "" {
		c.Env = v
	}
	if v := os.Getenv("PERMIT_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.Port = p
		}
	}
	if v := os.Getenv("PERMIT_ASSETS_DIR"); v != "" {
		c.AssetsDir = v
	}
	if v := os.Getenv("PERMIT_UPLOADS_DIR"); v != "" {
		c.UploadsDir = v
	}
	if v := os.Getenv("PERMIT_JWT_SECRET"); v != "" {
		c.JWTSecret = v
	}
	if v := os.Getenv("PERMIT_LOG_JSON"); v != "" {
		switch v {
		case "1", "true", "TRUE":
			c.LogJSON = true
		case "0", "false", "FALSE":
			c.LogJSON = false
		}
	}
	if v := os.Getenv("PERMIT_ALGO_URL"); v != "" {
		c.AlgoURL = v
	}
	return c
}
