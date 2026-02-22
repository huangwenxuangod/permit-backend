package usecase

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"permit-backend/internal/domain"
	"permit-backend/internal/infrastructure/zjzapi"
)

type TaskRepo interface {
	Put(*domain.Task) error
	Get(id string) (*domain.Task, bool)
}

type DownloadTokenRepo interface {
	PutToken(*domain.DownloadToken) error
	GetToken(token string) (*domain.DownloadToken, bool)
	UpdateToken(*domain.DownloadToken) error
}

type AssetWriter interface {
	Write(taskID, color string, data []byte) (string, error)
	WriteFile(taskID, filename string, data []byte) (string, error)
}

type ZJZClient interface {
	IDCardMake(ctx context.Context, itemID int, imageBase64 string, colors []string, enhance, beauty int) (zjzapi.IDCardResp, error)
	IDCardAll(ctx context.Context, itemID int, imageBase64 string, colors []string, enhance, beauty int) (zjzapi.IDCardResp, error)
}

type TaskService struct {
	Repo       TaskRepo
	Assets     AssetWriter
	ZJZ        ZJZClient
	UploadsDir string
	AssetsDir  string
	UseWatermark bool
}

type DownloadService struct {
	Repo  DownloadTokenRepo
	Tasks TaskRepo
}

func (s *TaskService) CreateTask(userID, specCode, sourceObjectKey string, itemID int, defaultBackground string, width, height, dpi int, availableColors []string, beauty, enhance int, useWatermark bool) (*domain.Task, error) {
	taskID := randomID()
	now := time.Now().UTC()
	t := &domain.Task{
		ID:              taskID,
		UserID:          userID,
		SpecCode:        specCode,
		Spec:            domain.TaskSpec{Code: specCode, WidthPx: width, HeightPx: height, DPI: dpi},
		ItemID:          itemID,
		Watermark:       useWatermark,
		Beauty:          beauty,
		Enhance:         enhance,
		SourceObjectKey: sourceObjectKey,
		Status:          domain.StatusProcessing,
		ProcessedUrls:   map[string]string{},
		LayoutUrls:      map[string]string{},
		AvailableColors: availableColors,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	_ = s.Repo.Put(t)
	srcPath := s.objectKeyToPath(sourceObjectKey)
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		t.Status = domain.StatusFailed
		t.ErrorMsg = "read source error"
		t.UpdatedAt = time.Now().UTC()
		_ = s.Repo.Put(t)
		return t, nil
	}
	imageB64 := base64.StdEncoding.EncodeToString(raw)
	if !useWatermark && s.UseWatermark {
		useWatermark = true
	}
	resp, err := s.callIDCard(context.Background(), itemID, imageB64, availableColors, enhance, beauty, useWatermark)
	if err != nil {
		t.Status = domain.StatusFailed
		t.ErrorMsg = "zjz idcard error: " + err.Error()
		t.UpdatedAt = time.Now().UTC()
		_ = s.Repo.Put(t)
		return t, nil
	}
	list := resp.Data.List
	if len(list) == 0 {
		t.Status = domain.StatusFailed
		t.ErrorMsg = "zjz idcard empty list"
		t.UpdatedAt = time.Now().UTC()
		_ = s.Repo.Put(t)
		return t, nil
	}
	colors := availableColors
	if len(colors) == 0 {
		colors = keysSorted(list)
		t.AvailableColors = colors
	}
	for _, c := range colors {
		u, ok := list[c]
		if !ok {
			continue
		}
		data, err := s.downloadImage(u)
		if err != nil {
			continue
		}
		url, err := s.Assets.Write(taskID, c, data)
		if err != nil {
			continue
		}
		t.ProcessedUrls[c] = url
	}
	if len(t.ProcessedUrls) == 0 {
		t.Status = domain.StatusFailed
		t.ErrorMsg = "zjz idcard download empty"
		t.UpdatedAt = time.Now().UTC()
		_ = s.Repo.Put(t)
		return t, nil
	}
	bgColor := strings.TrimSpace(defaultBackground)
	if bgColor == "" && len(colors) > 0 {
		bgColor = colors[0]
	}
	if u, ok := t.ProcessedUrls[bgColor]; ok {
		t.BaselineUrl = u
	}
	t.Status = domain.StatusDone
	t.UpdatedAt = time.Now().UTC()
	_ = s.Repo.Put(t)
	return t, nil
}

func (s *TaskService) GenerateBackground(taskID string, colorName string, dpi int) (string, error) {
	t, ok := s.Repo.Get(taskID)
	if !ok {
		return "", ErrNotFound("task")
	}
	if u, ok2 := t.ProcessedUrls[colorName]; ok2 && u != "" {
		return u, nil
	}
	srcPath := s.objectKeyToPath(t.SourceObjectKey)
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return "", err
	}
	imageB64 := base64.StdEncoding.EncodeToString(raw)
	resp, err := s.callIDCard(context.Background(), t.ItemID, imageB64, []string{colorName}, t.Enhance, t.Beauty, t.Watermark)
	if err != nil {
		return "", err
	}
	if len(resp.Data.List) == 0 {
		return "", ErrNotFound("zjz_idcard")
	}
	u, ok2 := resp.Data.List[colorName]
	if !ok2 {
		for k, v := range resp.Data.List {
			u = v
			colorName = k
			break
		}
	}
	if u == "" {
		return "", ErrNotFound("zjz_color")
	}
	jpg, err := s.downloadImage(u)
	if err != nil {
		return "", err
	}
	url, err := s.Assets.Write(taskID, colorName, jpg)
	if err != nil {
		return "", err
	}
	t.ProcessedUrls[colorName] = url
	t.UpdatedAt = time.Now().UTC()
	_ = s.Repo.Put(t)
	return url, nil
}

func (s *TaskService) GenerateLayout(taskID string, colorName string, width, height, dpi, kb int) (string, error) {
	t, ok := s.Repo.Get(taskID)
	if !ok {
		return "", ErrNotFound("task")
	}
	if t.LayoutUrls != nil {
		if u, ok2 := t.LayoutUrls["6inch"]; ok2 && u != "" {
			return u, nil
		}
	}
	if _, ok2 := t.ProcessedUrls[colorName]; !ok2 {
		bgURL, err := s.GenerateBackground(taskID, colorName, dpi)
		if err != nil || bgURL == "" {
			return "", err
		}
	}
	p := filepath.Join(s.AssetsDir, taskID, strings.ToLower(colorName)+".jpg")
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	if width == 0 {
		width = t.Spec.WidthPx
	}
	if height == 0 {
		height = t.Spec.HeightPx
	}
	if dpi == 0 {
		dpi = t.Spec.DPI
	}
	jpg, err := s.generateLayout6Inch(data, width, height, dpi, kb)
	if err != nil {
		return "", err
	}
	url, err := s.Assets.WriteFile(taskID, "layout_6inch.jpg", jpg)
	if err != nil {
		return "", err
	}
	if t.LayoutUrls == nil {
		t.LayoutUrls = map[string]string{}
	}
	t.LayoutUrls["6inch"] = url
	t.UpdatedAt = time.Now().UTC()
	_ = s.Repo.Put(t)
	return url, nil
}

func (s *TaskService) callIDCard(ctx context.Context, itemID int, imageBase64 string, colors []string, enhance, beauty int, useWatermark bool) (zjzapi.IDCardResp, error) {
	if useWatermark {
		return s.ZJZ.IDCardMake(ctx, itemID, imageBase64, colors, enhance, beauty)
	}
	return s.ZJZ.IDCardAll(ctx, itemID, imageBase64, colors, enhance, beauty)
}

func (s *TaskService) downloadImage(u string) ([]byte, error) {
	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, ErrNotFound("image")
	}
	return io.ReadAll(resp.Body)
}

func (s *TaskService) generateLayout6Inch(data []byte, width, height, dpi, kb int) ([]byte, error) {
	im, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	sheetW := int(float64(dpi) * 6.0)
	sheetH := int(float64(dpi) * 4.0)
	if sheetW <= 0 || sheetH <= 0 {
		return nil, ErrBadRequest("dpi")
	}
	gap := 20
	cols := (sheetW + gap) / (width + gap)
	rows := (sheetH + gap) / (height + gap)
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	totalW := cols*width + (cols-1)*gap
	totalH := rows*height + (rows-1)*gap
	startX := (sheetW - totalW) / 2
	startY := (sheetH - totalH) / 2
	canvas := image.NewRGBA(image.Rect(0, 0, sheetW, sheetH))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			x := startX + c*(width+gap)
			y := startY + r*(height+gap)
			rect := image.Rect(x, y, x+width, y+height)
			draw.Draw(canvas, rect, im, image.Point{}, draw.Over)
		}
	}
	quality := 85
	if kb > 0 && kb < 200 {
		quality = 70
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, canvas, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (s *TaskService) objectKeyToPath(objectKey string) string {
	if len(objectKey) >= 8 && objectKey[:8] == "uploads/" {
		return filepath.Join(s.UploadsDir, objectKey[8:])
	}
	return filepath.Join(s.UploadsDir, objectKey)
}

func keysSorted(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (s *DownloadService) CreateToken(taskID, userID string, ttlSeconds int) (*domain.DownloadToken, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, ErrBadRequest("taskId required")
	}
	if strings.TrimSpace(userID) == "" {
		return nil, ErrBadRequest("userId required")
	}
	t, ok := s.Tasks.Get(taskID)
	if !ok {
		return nil, ErrNotFound("task")
	}
	if t.Status != domain.StatusDone {
		return nil, ErrBadRequest("task not ready")
	}
	if strings.TrimSpace(t.UserID) != "" && strings.TrimSpace(t.UserID) != strings.TrimSpace(userID) {
		return nil, ErrBadRequest("task not owned")
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 600
	}
	now := time.Now().UTC()
	dt := &domain.DownloadToken{
		Token:     randomID(),
		TaskID:    taskID,
		UserID:    userID,
		Status:    domain.DownloadTokenActive,
		ExpiresAt: now.Add(time.Duration(ttlSeconds) * time.Second),
		CreatedAt: now,
	}
	_ = s.Repo.PutToken(dt)
	return dt, nil
}

func (s *DownloadService) UseToken(token string) (*domain.DownloadToken, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrBadRequest("token required")
	}
	dt, ok := s.Repo.GetToken(token)
	if !ok {
		return nil, ErrNotFound("token")
	}
	now := time.Now().UTC()
	if dt.Status != domain.DownloadTokenActive {
		return nil, ErrConflict("token not active")
	}
	if now.After(dt.ExpiresAt) {
		dt.Status = domain.DownloadTokenExpired
		dt.UsedAt = now
		_ = s.Repo.UpdateToken(dt)
		return nil, ErrBadRequest("token expired")
	}
	dt.Status = domain.DownloadTokenUsed
	dt.UsedAt = now
	_ = s.Repo.UpdateToken(dt)
	return dt, nil
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
