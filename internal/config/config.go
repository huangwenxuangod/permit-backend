package config

import (
	"os"
	"strconv"
)

type Config struct {
	Env             string
	Port            int
	AssetsDir       string
	AssetsPublicURL string
	UploadsDir      string
	JWTSecret       string
	LogJSON         bool
	ZJZBaseURL      string
	ZJZKey          string
	ZJZAccessToken  string
	ZJZWatermark    bool
	PayMock         bool
	WechatAppID     string
	WechatSecret    string
	WechatMchID     string
	WechatNotifyURL string
	WechatMchSerial string
	WechatAPIv3Key  string
	WechatPrivateKey string
	WechatPlatformCert string
	PostgresDSN     string
}

func Default() Config {
	return Config{
		Env:             "dev",
		Port:            5000,
		AssetsDir:       "./assets",
		AssetsPublicURL: "",
		UploadsDir:      "./uploads",
		JWTSecret:       "",
		LogJSON:         true,
		ZJZBaseURL:      "https://api.zjzapi.com",
		ZJZKey:          "",
		ZJZAccessToken:  "",
		ZJZWatermark:    false,
		PayMock:         true,
		WechatAppID:     "",
		WechatSecret:    "",
		WechatMchID:     "",
		WechatNotifyURL: "",
		WechatMchSerial: "",
		WechatAPIv3Key:  "",
		WechatPrivateKey: "",
		WechatPlatformCert: "",
		PostgresDSN:     "",
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
	if v := os.Getenv("PERMIT_ASSETS_PUBLIC_URL"); v != "" {
		c.AssetsPublicURL = v
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
	if v := os.Getenv("PERMIT_ZJZ_BASE_URL"); v != "" {
		c.ZJZBaseURL = v
	}
	if v := os.Getenv("PERMIT_ZJZ_KEY"); v != "" {
		c.ZJZKey = v
	}
	if v := os.Getenv("PERMIT_ZJZ_ACCESS_TOKEN"); v != "" {
		c.ZJZAccessToken = v
	}
	if v := os.Getenv("PERMIT_ZJZ_WATERMARK"); v != "" {
		switch v {
		case "1", "true", "TRUE":
			c.ZJZWatermark = true
		case "0", "false", "FALSE":
			c.ZJZWatermark = false
		}
	}
	if v := os.Getenv("PERMIT_PAY_MOCK"); v != "" {
		switch v {
		case "1", "true", "TRUE":
			c.PayMock = true
		case "0", "false", "FALSE":
			c.PayMock = false
		}
	}
	if v := os.Getenv("PERMIT_WECHAT_APPID"); v != "" {
		c.WechatAppID = v
	}
	if v := os.Getenv("PERMIT_WECHAT_SECRET"); v != "" {
		c.WechatSecret = v
	}
	if v := os.Getenv("PERMIT_WECHAT_MCHID"); v != "" {
		c.WechatMchID = v
	}
	if v := os.Getenv("PERMIT_WECHAT_NOTIFY_URL"); v != "" {
		c.WechatNotifyURL = v
	}
	if v := os.Getenv("PERMIT_WECHAT_MCH_SERIAL"); v != "" {
		c.WechatMchSerial = v
	}
	if v := os.Getenv("PERMIT_WECHAT_API_V3_KEY"); v != "" {
		c.WechatAPIv3Key = v
	}
	if v := os.Getenv("PERMIT_WECHAT_PRIVATE_KEY"); v != "" {
		c.WechatPrivateKey = v
	}
	if v := os.Getenv("PERMIT_WECHAT_PLATFORM_CERT"); v != "" {
		c.WechatPlatformCert = v
	}
	if v := os.Getenv("POSTGRES_DSN"); v != "" {
		c.PostgresDSN = v
	}
	return c
}
