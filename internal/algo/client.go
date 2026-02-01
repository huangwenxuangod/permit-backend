package algo

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

type IDPhotoResp struct {
	Status              int    `json:"status"`
	ImageBase64Standard string `json:"image_base64_standard"`
	ImageBase64HD       string `json:"image_base64_hd"`
}

type AddBackgroundResp struct {
	Status     int    `json:"status"`
	ImageBase64 string `json:"image_base64"`
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
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
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
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

func DecodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
