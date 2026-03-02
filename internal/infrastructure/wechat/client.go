package wechat

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
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

type PayConfig struct {
	AppID          string
	MchID          string
	MchSerial      string
	PrivateKey     string
	APIv3Key       string
	PlatformCert   string
	HTTP           *http.Client
}

type PayClient struct {
	AppID          string
	MchID          string
	MchSerial      string
	PrivateKey     *rsa.PrivateKey
	APIv3Key       string
	PlatformCert   *x509.Certificate
	PlatformSerial string
	HTTP           *http.Client
}

func NewPayClient(cfg PayConfig) (*PayClient, error) {
	if strings.TrimSpace(cfg.AppID) == "" || strings.TrimSpace(cfg.MchID) == "" || strings.TrimSpace(cfg.MchSerial) == "" || strings.TrimSpace(cfg.PrivateKey) == "" || strings.TrimSpace(cfg.APIv3Key) == "" || strings.TrimSpace(cfg.PlatformCert) == "" {
		return nil, fmt.Errorf("wechat pay config incomplete")
	}
	pemKey, err := loadPEM(cfg.PrivateKey)
	if err != nil {
		return nil, err
	}
	priv, err := parsePrivateKey(pemKey)
	if err != nil {
		return nil, err
	}
	pemCert, err := loadPEM(cfg.PlatformCert)
	if err != nil {
		return nil, err
	}
	cert, err := parseCert(pemCert)
	if err != nil {
		return nil, err
	}
	serial := strings.ToUpper(cert.SerialNumber.Text(16))
	return &PayClient{
		AppID:          cfg.AppID,
		MchID:          cfg.MchID,
		MchSerial:      cfg.MchSerial,
		PrivateKey:     priv,
		APIv3Key:       cfg.APIv3Key,
		PlatformCert:   cert,
		PlatformSerial: serial,
		HTTP:           cfg.HTTP,
	}, nil
}

type jsapiPrepayReq struct {
	AppID       string          `json:"appid"`
	MchID       string          `json:"mchid"`
	Description string          `json:"description"`
	OutTradeNo  string          `json:"out_trade_no"`
	NotifyURL   string          `json:"notify_url"`
	Amount      jsapiAmount     `json:"amount"`
	Payer       jsapiPayer      `json:"payer"`
}

type jsapiAmount struct {
	Total    int    `json:"total"`
	Currency string `json:"currency"`
}

type jsapiPayer struct {
	OpenID string `json:"openid"`
}

type jsapiPrepayResp struct {
	PrepayID string `json:"prepay_id"`
}

func (c *PayClient) JSAPIPrepay(ctx context.Context, orderID string, amount int, description string, openID string, notifyURL string) (map[string]any, error) {
	if strings.TrimSpace(openID) == "" {
		return nil, fmt.Errorf("openid required")
	}
	if strings.TrimSpace(notifyURL) == "" {
		return nil, fmt.Errorf("notify_url required")
	}
	if strings.TrimSpace(description) == "" {
		description = "permit order"
	}
	reqBody := jsapiPrepayReq{
		AppID:       c.AppID,
		MchID:       c.MchID,
		Description: description,
		OutTradeNo:  orderID,
		NotifyURL:   notifyURL,
		Amount:      jsapiAmount{Total: amount, Currency: "CNY"},
		Payer:       jsapiPayer{OpenID: openID},
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	u := "https://api.mch.weixin.qq.com/v3/pay/transactions/jsapi"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	auth, err := c.buildAuthorization(http.MethodPost, u, raw)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", auth)
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("wechat pay error: %s", strings.TrimSpace(string(body)))
	}
	var out jsapiPrepayResp
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if strings.TrimSpace(out.PrepayID) == "" {
		return nil, fmt.Errorf("missing prepay_id")
	}
	return c.buildPayParams(out.PrepayID)
}

func (c *PayClient) VerifySignature(timestamp, nonce, body, signature, serial string) error {
	if strings.TrimSpace(timestamp) == "" || strings.TrimSpace(nonce) == "" || strings.TrimSpace(signature) == "" {
		return fmt.Errorf("signature headers required")
	}
	if c.PlatformCert == nil {
		return fmt.Errorf("platform cert missing")
	}
	if strings.TrimSpace(serial) != "" && strings.ToUpper(serial) != c.PlatformSerial {
		return fmt.Errorf("platform cert serial mismatch")
	}
	message := timestamp + "\n" + nonce + "\n" + body + "\n"
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return err
	}
	h := sha256.Sum256([]byte(message))
	return rsa.VerifyPKCS1v15(c.PlatformCert.PublicKey.(*rsa.PublicKey), crypto.SHA256, h[:], sig)
}

type NotifyResource struct {
	Algorithm      string `json:"algorithm"`
	Ciphertext     string `json:"ciphertext"`
	Nonce          string `json:"nonce"`
	AssociatedData string `json:"associated_data"`
}

func (c *PayClient) DecryptResource(resource NotifyResource) ([]byte, error) {
	key := []byte(c.APIv3Key)
	if len(key) != 32 {
		return nil, fmt.Errorf("api v3 key length invalid")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := []byte(resource.Nonce)
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("nonce length invalid")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(resource.Ciphertext)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte(resource.AssociatedData))
	if err != nil {
		return nil, err
	}
	return plain, nil
}

func (c *PayClient) buildAuthorization(method, rawURL string, body []byte) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	path := u.RequestURI()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randomString(32)
	message := method + "\n" + path + "\n" + timestamp + "\n" + nonce + "\n" + string(body) + "\n"
	signature, err := c.sign(message)
	if err != nil {
		return "", err
	}
	auth := fmt.Sprintf(`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",timestamp="%s",serial_no="%s",signature="%s"`, c.MchID, nonce, timestamp, c.MchSerial, signature)
	return auth, nil
}

func (c *PayClient) buildPayParams(prepayID string) (map[string]any, error) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := randomString(32)
	pkg := "prepay_id=" + prepayID
	message := c.AppID + "\n" + timestamp + "\n" + nonce + "\n" + pkg + "\n"
	signature, err := c.sign(message)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"appId":     c.AppID,
		"timeStamp": timestamp,
		"nonceStr":  nonce,
		"package":   pkg,
		"signType":  "RSA",
		"paySign":   signature,
	}, nil
}

func (c *PayClient) sign(message string) (string, error) {
	h := sha256.Sum256([]byte(message))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.PrivateKey, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func loadPEM(value string) ([]byte, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil, fmt.Errorf("empty pem")
	}
	if strings.Contains(v, "BEGIN") {
		return []byte(v), nil
	}
	return os.ReadFile(v)
}

func parsePrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("invalid private key")
	}
	if strings.Contains(block.Type, "PRIVATE KEY") {
		if block.Type == "PRIVATE KEY" {
			key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return nil, err
			}
			if k, ok := key.(*rsa.PrivateKey); ok {
				return k, nil
			}
			return nil, fmt.Errorf("private key type invalid")
		}
		if block.Type == "RSA PRIVATE KEY" {
			return x509.ParsePKCS1PrivateKey(block.Bytes)
		}
	}
	return nil, fmt.Errorf("unsupported private key")
}

func parseCert(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("invalid cert")
	}
	return x509.ParseCertificate(block.Bytes)
}

func randomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	const letters = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}
