package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"crypto/rand"
	"encoding/hex"

	"permit-backend/internal/algo"
	"permit-backend/internal/config"
	"permit-backend/internal/tasks"
)

type Server struct {
	cfg   config.Config
	store *tasks.Store
	mux   *http.ServeMux
}

func New(cfg config.Config) *Server {
	s := &Server{
		cfg:   cfg,
		store: tasks.NewStore(),
		mux:   http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.cors(s.mux)
}

func (s *Server) routes() {
	fs := http.StripPrefix("/assets/", http.FileServer(http.Dir(s.cfg.AssetsDir)))
	s.mux.Handle("/assets/", fs)
	s.mux.HandleFunc("/api/upload", s.handleUpload)
	s.mux.HandleFunc("/api/tasks", s.handleCreateTask)
	s.mux.HandleFunc("/api/tasks/", s.handleGetTask)
	s.mux.HandleFunc("/api/download/", s.handleDownloadInfo)
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only POST accepted")
		return
	}
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "invalid multipart form")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "field 'file' required")
		return
	}
	defer f.Close()
	name := filepath.Base(hdr.Filename)
	if !validImageName(name) {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "only jpg/png allowed")
		return
	}
	id := randomID()
	outName := id + "_" + name
	outPath := filepath.Join(s.cfg.UploadsDir, outName)
	if err := os.MkdirAll(s.cfg.UploadsDir, 0o755); err != nil {
		s.err(w, r, http.StatusInternalServerError, "ServerError", "cannot create uploads dir")
		return
	}
	dst, err := os.Create(outPath)
	if err != nil {
		s.err(w, r, http.StatusInternalServerError, "ServerError", "cannot save file")
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, f); err != nil {
		s.err(w, r, http.StatusInternalServerError, "ServerError", "cannot write file")
		return
	}
	objKey := "uploads/" + outName
	s.json(w, r, http.StatusOK, map[string]string{"objectKey": objKey})
}

type createTaskReq struct {
	SpecCode        string   `json:"specCode"`
	SourceObjectKey string   `json:"sourceObjectKey"`
	Colors          []string `json:"colors"`
	WidthPx         int      `json:"widthPx"`
	HeightPx        int      `json:"heightPx"`
	DPI             int      `json:"dpi"`
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only POST accepted")
		return
	}
	var req createTaskReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "invalid json")
		return
	}
	if req.SourceObjectKey == "" {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "sourceObjectKey required")
		return
	}
	if req.WidthPx == 0 {
		req.WidthPx = 295
	}
	if req.HeightPx == 0 {
		req.HeightPx = 413
	}
	if req.DPI == 0 {
		req.DPI = 300
	}
	if len(req.Colors) == 0 {
		req.Colors = []string{"white", "blue"}
	}

	taskID := randomID()
	now := time.Now().UTC()
	t := &tasks.Task{
		ID:              taskID,
		UserID:          "dev-user",
		SpecCode:        orDefault(req.SpecCode, "passport"),
		SourceObjectKey: req.SourceObjectKey,
		Status:          tasks.StatusProcessing,
		ProcessedUrls:   map[string]string{},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	s.store.Put(t)

	srcPath := s.objectKeyToPath(req.SourceObjectKey)
	idp, err := algo.IDPhoto(s.cfg.AlgoURL, srcPath, req.HeightPx, req.WidthPx, req.DPI)
	if err != nil || !idp.OK {
		t.Status = tasks.StatusFailed
		if err != nil {
			t.ErrorMsg = "algo idphoto error: " + err.Error()
		} else {
			t.ErrorMsg = "algo idphoto resp not ok"
		}
		t.UpdatedAt = time.Now().UTC()
		s.store.Put(t)
		s.json(w, r, http.StatusOK, t)
		return
	}
	rgbaB64 := idp.ImageBase64Standard
	if rgbaB64 == "" {
		rgbaB64 = idp.ImageBase64HD
	}
	assetsDir := filepath.Join(s.cfg.AssetsDir, taskID)
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Status = tasks.StatusFailed
		t.ErrorMsg = "cannot create assets"
		t.UpdatedAt = time.Now().UTC()
		s.store.Put(t)
		s.json(w, r, http.StatusOK, t)
		return
	}
	for _, c := range req.Colors {
		colorHex := colorHexOf(c)
		bg, err := algo.AddBackgroundBase64(s.cfg.AlgoURL, rgbaB64, colorHex, req.DPI)
		if err != nil || !bg.OK {
			t.Status = tasks.StatusFailed
			if err != nil {
				t.ErrorMsg = "algo add_background error: " + err.Error()
			} else {
				t.ErrorMsg = "algo add_background resp not ok"
			}
			t.UpdatedAt = time.Now().UTC()
			s.store.Put(t)
			s.json(w, r, http.StatusOK, t)
			return
		}
		data, err := algo.DecodeBase64(bg.ImageBase64)
		if err != nil {
			t.Status = tasks.StatusFailed
			prefix := bg.ImageBase64
			if len(prefix) > 32 {
				prefix = prefix[:32]
			}
			t.ErrorMsg = "decode image error: " + prefix
			t.UpdatedAt = time.Now().UTC()
			s.store.Put(t)
			s.json(w, r, http.StatusOK, t)
			return
		}
		out := filepath.Join(assetsDir, c+".jpg")
		if err := os.WriteFile(out, data, 0o644); err != nil {
			t.Status = tasks.StatusFailed
			t.ErrorMsg = "write image error"
			t.UpdatedAt = time.Now().UTC()
			s.store.Put(t)
			s.json(w, r, http.StatusOK, t)
			return
		}
		t.ProcessedUrls[c] = "/assets/" + taskID + "/" + c + ".jpg"
	}
	t.Status = tasks.StatusDone
	t.UpdatedAt = time.Now().UTC()
	s.store.Put(t)
	s.json(w, r, http.StatusOK, t)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only GET accepted")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	if id == "" {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "task id required")
		return
	}
	t, ok := s.store.Get(id)
	if !ok {
		s.err(w, r, http.StatusNotFound, "NotFound", "task not found")
		return
	}
	s.json(w, r, http.StatusOK, t)
}

func (s *Server) handleDownloadInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only GET accepted")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/download/")
	if id == "" {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "task id required")
		return
	}
	t, ok := s.store.Get(id)
	if !ok || t.Status != tasks.StatusDone {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "task not ready")
		return
	}
	s.json(w, r, http.StatusOK, map[string]any{
		"taskId":    id,
		"urls":      t.ProcessedUrls,
		"expiresIn": 600,
	})
}

func (s *Server) objectKeyToPath(objectKey string) string {
	if strings.HasPrefix(objectKey, "uploads/") {
		return filepath.Join(s.cfg.UploadsDir, strings.TrimPrefix(objectKey, "uploads/"))
	}
	return filepath.Join(s.cfg.UploadsDir, objectKey)
}

func validImageName(name string) bool {
	n := strings.ToLower(name)
	return strings.HasSuffix(n, ".jpg") || strings.HasSuffix(n, ".jpeg") || strings.HasSuffix(n, ".png")
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = randRead(b)
	return hexEncode(b)
}

func randRead(b []byte) (int, error) {
	return rand.Read(b)
}

func hexEncode(b []byte) string {
	return hex.EncodeToString(b)
}

func orDefault(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

func (s *Server) err(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	reqID := ""
	if v := r.Header.Get("Idempotency-Key"); v != "" {
		reqID = v
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":      code,
			"message":   msg,
			"requestId": reqID,
		},
	})
}

func (s *Server) json(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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
