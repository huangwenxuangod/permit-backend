package usecase

import (
	"path/filepath"
	"time"
	"permit-backend/internal/domain"
	"permit-backend/internal/algo"
	"crypto/rand"
	"encoding/hex"
)

type TaskRepo interface {
	Put(*domain.Task) error
	Get(id string) (*domain.Task, bool)
}

type AssetWriter interface {
	Write(taskID, color string, data []byte) (string, error)
}

type AlgoClient interface {
	IDPhoto(baseURL, imagePath string, height, width, dpi int) (algo.IDPhotoResp, error)
	AddBackgroundBase64(baseURL, rgbaBase64, colorHex string, dpi int) (algo.AddBackgroundResp, error)
}

type TaskService struct {
	Repo       TaskRepo
	Assets     AssetWriter
	Algo       AlgoClient
	AlgoURL    string
	UploadsDir string
}

func (s *TaskService) CreateTask(userID, specCode, sourceObjectKey string, colors []string, width, height, dpi int, colorHexOf func(string) string) (*domain.Task, error) {
	taskID := randomID()
	now := time.Now().UTC()
	t := &domain.Task{
		ID:              taskID,
		UserID:          userID,
		SpecCode:        specCode,
		SourceObjectKey: sourceObjectKey,
		Status:          domain.StatusProcessing,
		ProcessedUrls:   map[string]string{},
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
	for _, c := range colors {
		colorHex := colorHexOf(c)
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
		url, err := s.Assets.Write(taskID, c, data)
		if err != nil {
			t.Status = domain.StatusFailed
			t.ErrorMsg = "write image error"
			t.UpdatedAt = time.Now().UTC()
			_ = s.Repo.Put(t)
			return t, nil
		}
		t.ProcessedUrls[c] = url
	}
	t.Status = domain.StatusDone
	t.UpdatedAt = time.Now().UTC()
	_ = s.Repo.Put(t)
	return t, nil
}

func (s *TaskService) objectKeyToPath(objectKey string) string {
	if len(objectKey) >= 8 && objectKey[:8] == "uploads/" {
		return filepath.Join(s.UploadsDir, objectKey[8:])
	}
	return filepath.Join(s.UploadsDir, objectKey)
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
