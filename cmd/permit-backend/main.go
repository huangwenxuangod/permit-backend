package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"permit-backend/internal/config"
	"permit-backend/internal/env"
	"permit-backend/internal/server"
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
		AlgoURL:    envDefaults.AlgoURL,
		PayMock:    envDefaults.PayMock,
		WechatAppID: envDefaults.WechatAppID,
		WechatMchID: envDefaults.WechatMchID,
		WechatNotifyURL: envDefaults.WechatNotifyURL,
	}

	ensureDir(cfg.AssetsDir)
	ensureDir(cfg.UploadsDir)

	b, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(b))

	srv := server.New(cfg)
	addr := fmt.Sprintf(":%d", cfg.Port)
	fmt.Printf("Listening on http://127.0.0.1:%d\n", cfg.Port)
	_ = http.ListenAndServe(addr, srv.Handler())
}

func ensureDir(p string) {
	if p == "" {
		return
	}
	if _, err := os.Stat(p); os.IsNotExist(err) {
		_ = os.MkdirAll(p, 0o755)
	}
}
