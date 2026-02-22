package zjzapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL     string
	Key         string
	AccessToken string
	HTTP        *http.Client
}

type IDCardData struct {
	PicID string            `json:"pic_id"`
	List  map[string]string `json:"list"`
}

type IDCardResp struct {
	Code int        `json:"code"`
	Msg  string     `json:"msg"`
	Data IDCardData `json:"data"`
}

type ReceiptResp struct {
	Code int        `json:"code"`
	Msg  string     `json:"msg"`
	Data IDCardData `json:"data"`
}

type AIPhotoMakeData struct {
	EstimatedTime int `json:"estimated_time"`
	PicID         int `json:"pic_id"`
}

type AIPhotoMakeResp struct {
	Code int            `json:"code"`
	Msg  string         `json:"msg"`
	Data AIPhotoMakeData `json:"data"`
}

type FaceEnhanceData struct {
	Image string `json:"image"`
}

type FaceEnhanceResp struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data FaceEnhanceData `json:"data"`
}

type ItemListData struct {
	List []Item `json:"list"`
}

type ItemListResp struct {
	Code int          `json:"code"`
	Msg  string       `json:"msg"`
	Data ItemListData `json:"data"`
}

type ItemGetResp struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data Item        `json:"data"`
}

type Item struct {
	ItemID    string `json:"item_id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	WidthPx   string `json:"width_px"`
	HeightPx  string `json:"height_px"`
	WidthMM   string `json:"width_mm"`
	HeightMM  string `json:"height_mm"`
	DPI       string `json:"dpi"`
	FileSize  string `json:"file_size_msg"`
}

type UserInfoResp struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data map[string]any `json:"data"`
}

type UserAppResp struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data map[string]any `json:"data"`
}

func (c *Client) IDCardMake(ctx context.Context, itemID int, imageBase64 string, colors []string, enhance, beauty int) (IDCardResp, error) {
	values := url.Values{}
	values.Set("key", c.Key)
	values.Set("item_id", strconv.Itoa(itemID))
	values.Set("image", imageBase64)
	if len(colors) > 0 {
		values.Set("colors", strings.Join(colors, ","))
	}
	if enhance >= 0 {
		values.Set("enhance", strconv.Itoa(enhance))
	}
	if beauty >= 0 {
		values.Set("beauty", strconv.Itoa(beauty))
	}
	var out IDCardResp
	if err := c.postForm(ctx, "/idcardv5/make", values, &out); err != nil {
		return out, err
	}
	if out.Code != 0 {
		return out, errors.New(out.Msg)
	}
	return out, nil
}

func (c *Client) IDCardAll(ctx context.Context, itemID int, imageBase64 string, colors []string, enhance, beauty int) (IDCardResp, error) {
	values := url.Values{}
	values.Set("key", c.Key)
	values.Set("item_id", strconv.Itoa(itemID))
	values.Set("image", imageBase64)
	if len(colors) > 0 {
		values.Set("colors", strings.Join(colors, ","))
	}
	if enhance >= 0 {
		values.Set("enhance", strconv.Itoa(enhance))
	}
	if beauty >= 0 {
		values.Set("beauty", strconv.Itoa(beauty))
	}
	var out IDCardResp
	if err := c.postForm(ctx, "/idcardv5/all", values, &out); err != nil {
		return out, err
	}
	if out.Code != 0 {
		return out, errors.New(out.Msg)
	}
	return out, nil
}

func (c *Client) ReceiptMake(ctx context.Context, itemID int, imageBase64 string) (ReceiptResp, error) {
	values := url.Values{}
	values.Set("key", c.Key)
	values.Set("item_id", strconv.Itoa(itemID))
	values.Set("image", imageBase64)
	var out ReceiptResp
	if err := c.postForm(ctx, "/receipt/make", values, &out); err != nil {
		return out, err
	}
	if out.Code != 0 {
		return out, errors.New(out.Msg)
	}
	return out, nil
}

func (c *Client) ReceiptSubmit(ctx context.Context, picID, noticeURL, param string) (map[string]any, error) {
	values := url.Values{}
	values.Set("key", c.Key)
	values.Set("pic_id", picID)
	values.Set("notice_url", noticeURL)
	if strings.TrimSpace(param) != "" {
		values.Set("param", param)
	}
	var out map[string]any
	if err := c.postForm(ctx, "/receipt/submit", values, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) AIPhotoMake(ctx context.Context, templateID string, images []string, noticeURL string) (AIPhotoMakeResp, error) {
	values := url.Values{}
	values.Set("key", c.Key)
	values.Set("template_id", templateID)
	values.Set("notice_url", noticeURL)
	for _, img := range images {
		values.Add("images[]", img)
	}
	var out AIPhotoMakeResp
	if err := c.postForm(ctx, "/ai-photo/make", values, &out); err != nil {
		return out, err
	}
	if out.Code != 0 {
		return out, errors.New(out.Msg)
	}
	return out, nil
}

func (c *Client) AIPhotoTemplates(ctx context.Context) (map[string]any, error) {
	values := url.Values{}
	values.Set("key", c.Key)
	var out map[string]any
	if err := c.postForm(ctx, "/ai-photo/templates", values, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) FaceEnhance(ctx context.Context, imageBase64, size string) (FaceEnhanceResp, error) {
	values := url.Values{}
	values.Set("key", c.Key)
	values.Set("image", imageBase64)
	if strings.TrimSpace(size) != "" {
		values.Set("size", size)
	}
	var out FaceEnhanceResp
	if err := c.postForm(ctx, "/face/enhance", values, &out); err != nil {
		return out, err
	}
	if out.Code != 0 {
		return out, errors.New(out.Msg)
	}
	return out, nil
}

func (c *Client) ItemList(ctx context.Context) (ItemListResp, error) {
	values := url.Values{}
	values.Set("key", c.Key)
	var out ItemListResp
	if err := c.postForm(ctx, "/item/list", values, &out); err != nil {
		return out, err
	}
	if out.Code != 0 {
		return out, errors.New(out.Msg)
	}
	return out, nil
}

func (c *Client) ItemGet(ctx context.Context, itemID int) (ItemGetResp, error) {
	values := url.Values{}
	values.Set("key", c.Key)
	values.Set("item_id", strconv.Itoa(itemID))
	var out ItemGetResp
	if err := c.postForm(ctx, "/item/get", values, &out); err != nil {
		return out, err
	}
	if out.Code != 0 {
		return out, errors.New(out.Msg)
	}
	return out, nil
}

func (c *Client) UserInfo(ctx context.Context, accessToken string) (UserInfoResp, error) {
	values := url.Values{}
	values.Set("access_token", accessToken)
	var out UserInfoResp
	if err := c.postForm(ctx, "/user/info", values, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) UserApp(ctx context.Context, accessToken, key string) (UserAppResp, error) {
	values := url.Values{}
	values.Set("access_token", accessToken)
	values.Set("key", key)
	var out UserAppResp
	if err := c.postForm(ctx, "/user/app", values, &out); err != nil {
		return out, err
	}
	return out, nil
}

func (c *Client) postForm(ctx context.Context, path string, values url.Values, out any) error {
	base := strings.TrimSpace(c.BaseURL)
	if base == "" {
		base = "https://api.zjzapi.com"
	}
	u := strings.TrimRight(base, "/") + path
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return errors.New(string(body))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}
