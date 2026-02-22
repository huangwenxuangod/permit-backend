package usecase

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"permit-backend/internal/domain"
	"permit-backend/internal/infrastructure/asset"
	"permit-backend/internal/infrastructure/zjzapi"
)

type fakeRepo struct {
	m map[string]*domain.Task
}

func (r *fakeRepo) Put(t *domain.Task) error {
	if r.m == nil {
		r.m = map[string]*domain.Task{}
	}
	cp := *t
	r.m[t.ID] = &cp
	return nil
}
func (r *fakeRepo) Get(id string) (*domain.Task, bool) {
	if r.m == nil {
		return nil, false
	}
	t, ok := r.m[id]
	return t, ok
}

type testZJZ struct {
	baseURL string
}

func (t testZJZ) IDCardMake(ctx context.Context, itemID int, imageBase64 string, colors []string, enhance, beauty int) (zjzapi.IDCardResp, error) {
	return t.reply(colors), nil
}

func (t testZJZ) IDCardAll(ctx context.Context, itemID int, imageBase64 string, colors []string, enhance, beauty int) (zjzapi.IDCardResp, error) {
	return t.reply(colors), nil
}

func (t testZJZ) reply(colors []string) zjzapi.IDCardResp {
	if len(colors) == 0 {
		colors = []string{"white", "blue"}
	}
	list := map[string]string{}
	for _, c := range colors {
		list[c] = t.baseURL + "/img/" + c + ".jpg"
	}
	return zjzapi.IDCardResp{
		Code: 0,
		Msg:  "ok",
		Data: zjzapi.IDCardData{List: list},
	}
}

func makeSampleJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 30, G: 144, B: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
	return buf.Bytes()
}

func TestTaskService_EndToEnd(t *testing.T) {
	uploadsDir := t.TempDir()
	assetsDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/img/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		img := makeSampleJPEG(120, 160)
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(img)
	}))
	defer server.Close()

	// Prepare uploaded source image
	src := makeSampleJPEG(120, 160)
	srcName := "source.jpg"
	srcPath := filepath.Join(uploadsDir, srcName)
	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	repo := &fakeRepo{}
	fs := asset.NewFSWriter(assetsDir, "")
	svc := &TaskService{
		Repo:       repo,
		Assets:     fs,
		ZJZ:        testZJZ{baseURL: server.URL},
		UploadsDir: uploadsDir,
		AssetsDir:  assetsDir,
	}

	available := []string{"white", "blue"}
	tk, err := svc.CreateTask("user-1", "cn_1inch", "uploads/"+srcName, 1, "white", 295, 413, 300, available, 0, 0, false)
	if err != nil {
		t.Fatalf("CreateTask error: %v", err)
	}
	if tk.Status != domain.StatusDone {
		t.Fatalf("task status not done: %s (error=%s)", tk.Status, tk.ErrorMsg)
	}
	if tk.BaselineUrl == "" {
		t.Fatalf("baseline url empty")
	}
	if tk.ProcessedUrls["white"] == "" {
		t.Fatalf("processed white url empty")
	}

	// Generate another background color
	urlBlue, err := svc.GenerateBackground(tk.ID, "blue", tk.Spec.DPI)
	if err != nil {
		t.Fatalf("GenerateBackground blue failed: %v", err)
	}
	if urlBlue == "" {
		t.Fatalf("blue url empty")
	}

	// Generate 6-inch layout
	urlLayout, err := svc.GenerateLayout(tk.ID, "white", tk.Spec.WidthPx, tk.Spec.HeightPx, tk.Spec.DPI, 200)
	if err != nil {
		t.Fatalf("GenerateLayout error: %v", err)
	}
	if urlLayout == "" {
		t.Fatalf("layout url empty")
	}
	if tk.LayoutUrls["6inch"] == "" {
		t.Fatalf("layoutUrls missing 6inch")
	}

	// Verify asset files exist
	layoutPath := filepath.Join(assetsDir, tk.ID, "layout_6inch.jpg")
	if _, err := os.Stat(layoutPath); err != nil {
		t.Fatalf("layout file not found: %v", err)
	}
	whitePath := filepath.Join(assetsDir, tk.ID, "white.jpg")
	if _, err := os.Stat(whitePath); err != nil {
		t.Fatalf("white background file not found: %v", err)
	}

	// Ensure updated timestamp moved forward
	if time.Since(tk.UpdatedAt) > time.Minute {
		t.Fatalf("updatedAt not recent: %v", tk.UpdatedAt)
	}
}
