package usecase

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"permit-backend/internal/domain"
	"permit-backend/internal/infrastructure/asset"
	"permit-backend/internal/algo"
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

type testAlgo struct{}

func (testAlgo) IDPhoto(baseURL, imagePath string, height, width, dpi int) (algo.IDPhotoResp, error) {
	img := image.NewRGBA(image.Rect(0, 0, max(width, 100), max(height, 100)))
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 200, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())
	return algo.IDPhotoResp{OK: true, ImageBase64Standard: "data:image/png;base64," + b64}, nil
}
func (testAlgo) AddBackgroundBase64(baseURL, rgbaBase64, colorHex string, dpi int) (algo.AddBackgroundResp, error) {
	data, err := algo.DecodeBase64(rgbaBase64)
	if err != nil {
		return algo.AddBackgroundResp{}, err
	}
	im, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return algo.AddBackgroundResp{}, err
	}
	var out bytes.Buffer
	_ = jpeg.Encode(&out, im, &jpeg.Options{Quality: 85})
	b64 := base64.StdEncoding.EncodeToString(out.Bytes())
	return algo.AddBackgroundResp{OK: true, ImageBase64: b64}, nil
}
func (testAlgo) AddBackgroundFile(baseURL string, rgbaPNG []byte, colorHex string, dpi int) (algo.AddBackgroundResp, error) {
	im, err := png.Decode(bytes.NewReader(rgbaPNG))
	if err != nil {
		return algo.AddBackgroundResp{}, err
	}
	var out bytes.Buffer
	_ = jpeg.Encode(&out, im, &jpeg.Options{Quality: 85})
	b64 := base64.StdEncoding.EncodeToString(out.Bytes())
	return algo.AddBackgroundResp{OK: true, ImageBase64: b64}, nil
}
func (testAlgo) GenerateLayoutPhotosFile(baseURL string, rgbImage []byte, height, width, dpi, kb int) (algo.LayoutResp, error) {
	// Pass-through: return the given RGB image as the layout
	b64 := base64.StdEncoding.EncodeToString(rgbImage)
	return algo.LayoutResp{OK: true, ImageBase64: b64}, nil
}

func colorHexOf(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "white":
		return "ffffff"
	case "blue":
		return "638cce"
	case "red":
		return "ff0000"
	default:
		return "ffffff"
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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

	// Prepare uploaded source image
	src := makeSampleJPEG(120, 160)
	srcName := "source.jpg"
	srcPath := filepath.Join(uploadsDir, srcName)
	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	repo := &fakeRepo{}
	fs := asset.NewFSWriter(assetsDir)
	al := testAlgo{}
	svc := &TaskService{
		Repo:       repo,
		Assets:     fs,
		Algo:       al,
		AlgoURL:    "http://127.0.0.1:8080",
		UploadsDir: uploadsDir,
		AssetsDir:  assetsDir,
	}

	available := []string{"white", "blue"}
	tk, err := svc.CreateTask("user-1", "cn_1inch", "uploads/"+srcName, "white", 295, 413, 300, available, colorHexOf)
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
	urlBlue, err := svc.GenerateBackground(tk.ID, "blue", tk.Spec.DPI, colorHexOf)
	if err != nil {
		t.Fatalf("GenerateBackground blue failed: %v", err)
	}
	if urlBlue == "" {
		t.Fatalf("blue url empty")
	}

	// Generate 6-inch layout
	urlLayout, err := svc.GenerateLayout(tk.ID, "white", tk.Spec.WidthPx, tk.Spec.HeightPx, tk.Spec.DPI, 200, colorHexOf)
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

