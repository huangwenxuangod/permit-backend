package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"permit-backend/internal/config"
	"permit-backend/internal/env"
)

func main() {
	env.Load(".env", ".env.local")
	envDefaults := config.EnvDefaults()

	env := flag.String("env", envDefaults.Env, "")
	port := flag.Int("port", envDefaults.Port, "")
	assets := flag.String("assets", envDefaults.AssetsDir, "")
	uploads := flag.String("uploads", envDefaults.UploadsDir, "")
	jwtSecret := flag.String("jwt-secret", envDefaults.JWTSecret, "")
	logJSON := flag.Bool("log-json", envDefaults.LogJSON, "")

	flag.Parse()

	cfg := config.Config{
		Env:        *env,
		Port:       *port,
		AssetsDir:  *assets,
		UploadsDir: *uploads,
		JWTSecret:  *jwtSecret,
		LogJSON:    *logJSON,
	}

	ensureDir(cfg.AssetsDir)
	ensureDir(cfg.UploadsDir)

	b, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(b))
}

func ensureDir(p string) {
	if p == "" {
		return
	}
	if _, err := os.Stat(p); os.IsNotExist(err) {
		_ = os.MkdirAll(p, 0o755)
	}
}
