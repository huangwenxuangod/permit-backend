package usecase

import (
	"os"
	"path/filepath"
	"time"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"permit-backend/internal/domain"
	"permit-backend/internal/algo"
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

type AlgoClient interface {
	IDPhoto(baseURL, imagePath string, height, width, dpi int) (algo.IDPhotoResp, error)
	AddBackgroundBase64(baseURL, rgbaBase64, colorHex string, dpi int) (algo.AddBackgroundResp, error)
	AddBackgroundFile(baseURL string, rgbaPNG []byte, colorHex string, dpi int) (algo.AddBackgroundResp, error)
	GenerateLayoutPhotosFile(baseURL string, rgbImage []byte, height, width, dpi, kb int) (algo.LayoutResp, error)
}

type TaskService struct {
	Repo       TaskRepo
	Assets     AssetWriter
	Algo       AlgoClient
	AlgoURL    string
	UploadsDir string
	AssetsDir  string
}

type DownloadService struct {
	Repo  DownloadTokenRepo
	Tasks TaskRepo
}

func (s *TaskService) CreateTask(userID, specCode, sourceObjectKey string, defaultBackground string, width, height, dpi int, availableColors []string, colorHexOf func(string) string) (*domain.Task, error) {
	taskID := randomID()
	now := time.Now().UTC()
	t := &domain.Task{
		ID:              taskID,
		UserID:          userID,
		SpecCode:        specCode,
		Spec:            domain.TaskSpec{Code: specCode, WidthPx: width, HeightPx: height, DPI: dpi},
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
	idp, err := s.Algo.IDPhoto(s.AlgoURL, srcPath, height, width, dpi)
	if err != nil || !idp.OK {
		t.Status = domain.StatusFailed
		if err != nil {
			t.ErrorMsg = "algo idphoto error: " + err.Error()
		} else {
			t.ErrorMsg = "algo idphoto resp not ok"
		}
		t.UpdatedAt = time.Now().UTC()
		_ = s.Repo.Put(t)
		return t, nil
	}
	rgbaB64 := idp.ImageBase64Standard
	if rgbaB64 == "" {
		rgbaB64 = idp.ImageBase64HD
	}
	rgbaData, err := algo.DecodeBase64(rgbaB64)
	if err != nil {
		t.Status = domain.StatusFailed
		prefix := rgbaB64
		if len(prefix) > 32 {
			prefix = prefix[:32]
		}
		t.ErrorMsg = "decode baseline error: " + prefix
		t.UpdatedAt = time.Now().UTC()
		_ = s.Repo.Put(t)
		return t, nil
	}
	baseURL, err := s.Assets.WriteFile(taskID, "baseline.png", rgbaData)
	if err != nil {
		t.Status = domain.StatusFailed
		t.ErrorMsg = "write baseline error"
		t.UpdatedAt = time.Now().UTC()
		_ = s.Repo.Put(t)
		return t, nil
	}
	t.BaselineUrl = baseURL

	bgColor := defaultBackground
	if bgColor == "" {
		bgColor = "white"
	}
	colorHex := colorHexOf(bgColor)
	bg, err := s.Algo.AddBackgroundBase64(s.AlgoURL, rgbaB64, colorHex, dpi)
	if err != nil || !bg.OK {
		t.Status = domain.StatusFailed
		if err != nil {
			t.ErrorMsg = "algo add_background error: " + err.Error()
		} else {
			t.ErrorMsg = "algo add_background resp not ok"
		}
		t.UpdatedAt = time.Now().UTC()
		_ = s.Repo.Put(t)
		return t, nil
	}
	data, err := algo.DecodeBase64(bg.ImageBase64)
	if err != nil {
		t.Status = domain.StatusFailed
		prefix := bg.ImageBase64
		if len(prefix) > 32 {
			prefix = prefix[:32]
		}
		t.ErrorMsg = "decode image error: " + prefix
		t.UpdatedAt = time.Now().UTC()
		_ = s.Repo.Put(t)
		return t, nil
	}
	url, err := s.Assets.Write(taskID, bgColor, data)
	if err != nil {
		t.Status = domain.StatusFailed
		t.ErrorMsg = "write image error"
		t.UpdatedAt = time.Now().UTC()
		_ = s.Repo.Put(t)
		return t, nil
	}
	t.ProcessedUrls[bgColor] = url

	t.Status = domain.StatusDone
	t.UpdatedAt = time.Now().UTC()
	_ = s.Repo.Put(t)
	return t, nil
}

func (s *TaskService) GenerateBackground(taskID string, colorName string, dpi int, colorHexOf func(string) string) (string, error) {
	t, ok := s.Repo.Get(taskID)
	if !ok {
		return "", ErrNotFound("task")
	}
	if u, ok2 := t.ProcessedUrls[colorName]; ok2 && u != "" {
		return u, nil
	}
	p := filepath.Join(s.AssetsDir, taskID, "baseline.png")
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	colorHex := colorHexOf(colorName)
	bg, err := s.Algo.AddBackgroundFile(s.AlgoURL, data, colorHex, dpi)
	if err != nil || !bg.OK {
		if err != nil {
			return "", err
		}
		return "", ErrNotFound("add_background")
	}
	jpg, err := algo.DecodeBase64(bg.ImageBase64)
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

func (s *TaskService) GenerateLayout(taskID string, colorName string, width, height, dpi, kb int, colorHexOf func(string) string) (string, error) {
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
		bgURL, err := s.GenerateBackground(taskID, colorName, dpi, colorHexOf)
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
	println("GenerateLayout size:", width, height, dpi, "kb", kb, "color", colorName)
	resp, err := s.Algo.GenerateLayoutPhotosFile(s.AlgoURL, data, height, width, dpi, kb)
	if err != nil || !resp.OK {
		return "", err
	}
	jpg, err := algo.DecodeBase64(resp.ImageBase64)
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

func (s *TaskService) objectKeyToPath(objectKey string) string {
	if len(objectKey) >= 8 && objectKey[:8] == "uploads/" {
		return filepath.Join(s.UploadsDir, objectKey[8:])
	}
	return filepath.Join(s.UploadsDir, objectKey)
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
