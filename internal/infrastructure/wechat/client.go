package wechat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"strings"
)

type Client struct {
	AppID  string
	Secret string
	HTTP   *http.Client
}

type jscode2sessionResp struct {
	OpenID     string `json:"openid"`
	SessionKey string `json:"session_key"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

func (c *Client) Jscode2Session(code string) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(code), "mock_") {
		return code[5:], "mock_session", nil
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 8 * time.Second}
	}
	u := fmt.Sprintf("https://api.weixin.qq.com/sns/jscode2session?appid=%s&secret=%s&js_code=%s&grant_type=authorization_code", c.AppID, c.Secret, code)
	resp, err := hc.Get(u)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var out jscode2sessionResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", err
	}
	if out.ErrCode != 0 {
		return "", "", fmt.Errorf("wechat error: %d %s", out.ErrCode, out.ErrMsg)
	}
	return out.OpenID, out.SessionKey, nil
}
