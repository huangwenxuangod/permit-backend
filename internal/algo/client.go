package algo

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type IDPhotoResp struct {
	OK                   bool
	ImageBase64Standard  string
	ImageBase64HD        string
}

type AddBackgroundResp struct {
	OK         bool
	ImageBase64 string
}

type LayoutResp struct {
	OK          bool
	ImageBase64 string
}

func AddBackgroundFile(baseURL string, rgbaPNG []byte, colorHex string, dpi int) (AddBackgroundResp, error) {
	var out AddBackgroundResp
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile("input_image", "rgba.png")
	if err != nil {
		return out, err
	}
	if _, err = fw.Write(rgbaPNG); err != nil {
		return out, err
	}
	_ = w.WriteField("color", colorHex)
	_ = w.WriteField("dpi", itoa(dpi))
	if err := w.Close(); err != nil {
		return out, err
	}
	req, err := http.NewRequest("POST", baseURL+"/add_background", body)
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	var m map[string]any
	if err = json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return out, err
	}
	out.ImageBase64, _ = mString(m["image_base64"])
	out.OK = parseStatus(m["status"])
	return out, nil
}

func IDPhoto(baseURL, imagePath string, height, width, dpi int) (IDPhotoResp, error) {
	var out IDPhotoResp
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	f, err := os.Open(imagePath)
	if err != nil {
		return out, err
	}
	defer f.Close()
	fw, err := w.CreateFormFile("input_image", filepath.Base(imagePath))
	if err != nil {
		return out, err
	}
	if _, err = io.Copy(fw, f); err != nil {
		return out, err
	}
	_ = w.WriteField("height", itoa(height))
	_ = w.WriteField("width", itoa(width))
	_ = w.WriteField("hd", "true")
	_ = w.WriteField("dpi", itoa(dpi))
	_ = w.WriteField("face_alignment", "true")
	if err = w.Close(); err != nil {
		return out, err
	}
	req, err := http.NewRequest("POST", baseURL+"/idphoto", body)
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	var m map[string]any
	if err = json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return out, err
	}
	out.ImageBase64Standard, _ = mString(m["image_base64_standard"])
	out.ImageBase64HD, _ = mString(m["image_base64_hd"])
	out.OK = parseStatus(m["status"])
	return out, nil
}

func AddBackgroundBase64(baseURL, rgbaBase64, colorHex string, dpi int) (AddBackgroundResp, error) {
	var out AddBackgroundResp
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("input_image_base64", rgbaBase64)
	_ = w.WriteField("color", colorHex)
	_ = w.WriteField("dpi", itoa(dpi))
	if err := w.Close(); err != nil {
		return out, err
	}
	req, err := http.NewRequest("POST", baseURL+"/add_background", body)
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	var m map[string]any
	if err = json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return out, err
	}
	out.ImageBase64, _ = mString(m["image_base64"])
	out.OK = parseStatus(m["status"])
	return out, nil
}

func GenerateLayoutPhotosFile(baseURL string, rgbImage []byte, height, width, dpi, kb int) (LayoutResp, error) {
	var out LayoutResp
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_, _ = io.WriteString(body, "")
	fmt.Printf("GenerateLayoutPhotos request: len=%d height=%d width=%d dpi=%d kb=%d\n", len(rgbImage), height, width, dpi, kb)
	fw, err := w.CreateFormFile("input_image", "input.jpg")
	if err != nil {
		return out, err
	}
	if _, err = fw.Write(rgbImage); err != nil {
		return out, err
	}
	_ = w.WriteField("height", itoa(height))
	_ = w.WriteField("width", itoa(width))
	if kb > 0 {
		_ = w.WriteField("kb", itoa(kb))
	}
	_ = w.WriteField("dpi", itoa(dpi))
	if err := w.Close(); err != nil {
		return out, err
	}
	req, err := http.NewRequest("POST", baseURL+"/generate_layout_photos", body)
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, io.ErrUnexpectedEOF
	}
	var m map[string]any
	if err = json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return out, err
	}
	out.ImageBase64, _ = mString(m["image_base64"])
	out.OK = parseStatus(m["status"])
	return out, nil
}

func DecodeBase64(s string) ([]byte, error) {
	if i := strings.Index(s, "base64,"); i >= 0 {
		s = s[i+7:]
	}
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	if rem := len(s) % 4; rem != 0 {
		s = s + strings.Repeat("=", 4-rem)
	}
	return base64.StdEncoding.DecodeString(s)
}

func itoa(i int) string {
	return strconv.Itoa(i)
}

func mString(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	default:
		return "", false
	}
}

func parseStatus(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case int:
		return t != 0
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i != 0
		}
		return false
	default:
		return false
	}
}
