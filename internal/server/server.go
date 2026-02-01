package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"crypto/rand"
	"encoding/hex"
	"strconv"

	"permit-backend/internal/algo"
	"permit-backend/internal/config"
	"permit-backend/internal/domain"
	"permit-backend/internal/usecase"
	"permit-backend/internal/infrastructure/asset"
	"permit-backend/internal/infrastructure/repo"
)

type Server struct {
	cfg   config.Config
	mux   *http.ServeMux
	taskSvc  *usecase.TaskService
	orderSvc *usecase.OrderService
}

func New(cfg config.Config) *Server {
	s := &Server{cfg: cfg, mux: http.NewServeMux()}

	var taskRepo usecase.TaskRepo
	var orderRepo usecase.OrderRepo

	if strings.TrimSpace(cfg.PostgresDSN) != "" {
		pg, err := repo.NewPostgresRepo(cfg.PostgresDSN)
		if err == nil {
			taskRepo = pg
			orderRepo = &pgOrderRepo{pg: pg}
		}
	}
	if taskRepo == nil {
		taskRepo = repo.NewMemoryTaskRepo()
	}
	if orderRepo == nil {
		orderRepo = repo.NewMemoryOrderRepo()
	}

	fs := asset.NewFSWriter(cfg.AssetsDir)
	al := algoAdapter{}

	s.taskSvc = &usecase.TaskService{
		Repo:       taskRepo,
		Assets:     fs,
		Algo:       al,
		AlgoURL:    cfg.AlgoURL,
		UploadsDir: cfg.UploadsDir,
	}
	s.orderSvc = &usecase.OrderService{
		Repo:        orderRepo,
		PayMock:     cfg.PayMock,
		WechatAppID: cfg.WechatAppID,
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
	s.mux.HandleFunc("/api/specs", s.handleSpecs)
	s.mux.HandleFunc("/api/upload", s.handleUpload)
	s.mux.HandleFunc("/api/tasks", s.handleCreateTask)
	s.mux.HandleFunc("/api/tasks/", s.handleGetTask)
	s.mux.HandleFunc("/api/download/", s.handleDownloadInfo)
	s.mux.HandleFunc("/api/orders", s.handleOrders)
	s.mux.HandleFunc("/api/orders/", s.handleGetOrder)
	s.mux.HandleFunc("/api/pay/wechat", s.handlePayWechat)
	s.mux.HandleFunc("/api/pay/douyin", s.handlePayDouyin)
	s.mux.HandleFunc("/api/pay/callback", s.handlePayCallback)
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

type Spec struct {
	Code     string   `json:"code"`
	Name     string   `json:"name"`
	WidthPx  int      `json:"widthPx"`
	HeightPx int      `json:"heightPx"`
	DPI      int      `json:"dpi"`
	BgColors []string `json:"bgColors"`
}

type createOrderReq struct {
	TaskID      string      `json:"taskId"`
	Items       []domain.OrderItem `json:"items"`
	City        string      `json:"city"`
	Remark      string      `json:"remark"`
	AmountCents int         `json:"amountCents"`
	Channel     string      `json:"channel"`
}

func (s *Server) handleSpecs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only GET accepted")
		return
	}
	s.json(w, r, http.StatusOK, s.defaultSpecs())
}

func (s *Server) defaultSpecs() []Spec {
	return []Spec{
		{Code: "passport", Name: "护照", WidthPx: 354, HeightPx: 472, DPI: 300, BgColors: []string{"white", "blue", "red"}},
		{Code: "cn_1inch", Name: "一寸", WidthPx: 295, HeightPx: 413, DPI: 300, BgColors: []string{"white", "blue", "red"}},
		{Code: "cn_2inch", Name: "二寸", WidthPx: 413, HeightPx: 626, DPI: 300, BgColors: []string{"white", "blue", "red"}},
	}
}

func (s *Server) findSpec(code string) Spec {
	code = strings.TrimSpace(strings.ToLower(code))
	specs := s.defaultSpecs()
	for _, sp := range specs {
		if strings.ToLower(sp.Code) == code {
			return sp
		}
	}
	return specs[0]
}

func (s *Server) handleOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var req createOrderReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.err(w, r, http.StatusBadRequest, "BadRequest", "invalid json")
			return
		}
		if req.TaskID == "" {
			s.err(w, r, http.StatusBadRequest, "BadRequest", "taskId required")
			return
		}
		if _, ok := s.taskSvc.Repo.Get(req.TaskID); !ok {
			s.err(w, r, http.StatusBadRequest, "BadRequest", "task not found")
			return
		}
		o := &domain.Order{
			TaskID:      req.TaskID,
			Items:       req.Items,
			City:        req.City,
			Remark:      req.Remark,
			AmountCents: req.AmountCents,
			Channel:     orDefault(req.Channel, "wechat"),
		}
		id, _ := s.orderSvc.Create(o)
		s.json(w, r, http.StatusOK, map[string]any{"orderId": id, "status": string(o.Status)})
		return
	}
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		page := 1
		pageSize := 20
		if v := q.Get("page"); v != "" {
			if i, err := strconv.Atoi(v); err == nil && i > 0 {
				page = i
			}
		}
		if v := q.Get("pageSize"); v != "" {
			if i, err := strconv.Atoi(v); err == nil && i > 0 {
				pageSize = i
			}
		}
		items, total := s.orderSvc.Repo.List(page, pageSize)
		s.json(w, r, http.StatusOK, map[string]any{"items": items, "page": page, "pageSize": pageSize, "total": total})
		return
	}
	s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only GET/POST accepted")
}

func (s *Server) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only GET accepted")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/orders/")
	if id == "" {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "order id required")
		return
	}
	o, ok := s.orderSvc.Repo.Get(id)
	if !ok {
		s.err(w, r, http.StatusNotFound, "NotFound", "order not found")
		return
	}
	s.json(w, r, http.StatusOK, o)
}

type payReq struct {
	OrderID string `json:"orderId"`
}

func (s *Server) handlePayWechat(w http.ResponseWriter, r *http.Request) {
	s.handlePay(w, r, "wechat")
}

func (s *Server) handlePayDouyin(w http.ResponseWriter, r *http.Request) {
	s.handlePay(w, r, "douyin")
}

func (s *Server) handlePay(w http.ResponseWriter, r *http.Request, channel string) {
	if r.Method != http.MethodPost {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only POST accepted")
		return
	}
	var req payReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "invalid json")
		return
	}
	if req.OrderID == "" {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "orderId required")
		return
	}
	if !s.cfg.PayMock {
		s.err(w, r, http.StatusNotImplemented, "NotImplemented", "real payment not configured")
		return
	}
	p, err := s.orderSvc.Pay(req.OrderID, channel)
	if err != nil {
		s.err(w, r, http.StatusNotFound, "NotFound", "order not found")
		return
	}
	s.json(w, r, http.StatusOK, map[string]any{"orderId": req.OrderID, "payParams": p})
}

type payCallbackReq struct {
	OrderID     string `json:"orderId"`
	Status      string `json:"status"`
	Raw         string `json:"raw"`
	SignatureOK bool   `json:"signature_ok"`
}

func (s *Server) handlePayCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only POST accepted")
		return
	}
	var req payCallbackReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "invalid json")
		return
	}
	if req.OrderID == "" {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "orderId required")
		return
	}
	_ = s.orderSvc.Callback(req.OrderID, strings.ToLower(req.Status))
	s.json(w, r, http.StatusOK, map[string]any{"ok": true})
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
	spec := s.findSpec(orDefault(req.SpecCode, "passport"))
	if req.WidthPx == 0 {
		req.WidthPx = spec.WidthPx
	}
	if req.HeightPx == 0 {
		req.HeightPx = spec.HeightPx
	}
	if req.DPI == 0 {
		req.DPI = spec.DPI
	}
	if len(req.Colors) == 0 {
		req.Colors = spec.BgColors
	}
	t, _ := s.taskSvc.CreateTask("dev-user", orDefault(req.SpecCode, "passport"), req.SourceObjectKey, req.Colors, req.WidthPx, req.HeightPx, req.DPI, colorHexOf)
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
	t, ok := s.taskSvc.Repo.Get(id)
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
	t, ok := s.taskSvc.Repo.Get(id)
	if !ok || t.Status != domain.StatusDone {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "task not ready")
		return
	}
	s.json(w, r, http.StatusOK, map[string]any{
		"taskId":    id,
		"urls":      t.ProcessedUrls,
		"expiresIn": 600,
	})
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

type algoAdapter struct{}

func (algoAdapter) IDPhoto(baseURL, imagePath string, height, width, dpi int) (algo.IDPhotoResp, error) {
	return algo.IDPhoto(baseURL, imagePath, height, width, dpi)
}
func (algoAdapter) AddBackgroundBase64(baseURL, rgbaBase64, colorHex string, dpi int) (algo.AddBackgroundResp, error) {
	return algo.AddBackgroundBase64(baseURL, rgbaBase64, colorHex, dpi)
}

type pgOrderRepo struct{ pg *repo.PostgresRepo }

func (p *pgOrderRepo) Put(o *domain.Order) error { return p.pg.PutOrder(o) }
func (p *pgOrderRepo) Get(id string) (*domain.Order, bool) { return p.pg.GetOrder(id) }
func (p *pgOrderRepo) List(page, pageSize int) ([]domain.Order, int) { return p.pg.ListOrders(page, pageSize) }
