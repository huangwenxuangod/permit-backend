package server

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"permit-backend/internal/algo"
	"permit-backend/internal/config"
	"permit-backend/internal/domain"
	"permit-backend/internal/infrastructure/asset"
	"permit-backend/internal/infrastructure/repo"
	"permit-backend/internal/infrastructure/wechat"
	"permit-backend/internal/usecase"
)

type Server struct {
	cfg      config.Config
	engine   *gin.Engine
	taskSvc  *usecase.TaskService
	orderSvc *usecase.OrderService
	authSvc  *usecase.AuthService
	downloadSvc *usecase.DownloadService
	pg       *repo.PostgresRepo
}

func New(cfg config.Config) *Server {
	s := &Server{cfg: cfg}

	var taskRepo usecase.TaskRepo
	var orderRepo usecase.OrderRepo
	var userRepo usecase.UserRepo
	var downloadRepo usecase.DownloadTokenRepo

	if strings.TrimSpace(cfg.PostgresDSN) != "" {
		pg, err := repo.NewPostgresRepo(cfg.PostgresDSN)
		if err == nil {
			taskRepo = pg
			orderRepo = &pgOrderRepo{pg: pg}
			userRepo = pg
			downloadRepo = pg
			s.pg = pg
		}
	}
	if taskRepo == nil {
		taskRepo = repo.NewMemoryTaskRepo()
	}
	if orderRepo == nil {
		orderRepo = repo.NewMemoryOrderRepo()
	}
	if userRepo == nil {
		userRepo = repo.NewMemoryUserRepo()
	}
	if downloadRepo == nil {
		downloadRepo = repo.NewMemoryDownloadTokenRepo()
	}

	fs := asset.NewFSWriter(cfg.AssetsDir, cfg.AssetsPublicURL)
	al := algoAdapter{}

	s.taskSvc = &usecase.TaskService{
		Repo:       taskRepo,
		Assets:     fs,
		Algo:       al,
		AlgoURL:    cfg.AlgoURL,
		UploadsDir: cfg.UploadsDir,
		AssetsDir:  cfg.AssetsDir,
	}
	s.orderSvc = &usecase.OrderService{
		Repo:        orderRepo,
		PayMock:     cfg.PayMock,
		WechatAppID: cfg.WechatAppID,
	}
	s.downloadSvc = &usecase.DownloadService{
		Repo:  downloadRepo,
		Tasks: taskRepo,
	}
	wc := &wechat.Client{AppID: cfg.WechatAppID, Secret: cfg.WechatSecret}
	s.authSvc = &usecase.AuthService{Repo: userRepo, Wechat: wc, JWTSecret: cfg.JWTSecret}
	s.engine = gin.New()
	s.engine.Use(func(c *gin.Context) {
		reqID := strings.TrimSpace(c.GetHeader("X-Request-Id"))
		if reqID == "" {
			reqID = randomID()
		}
		c.Request.Header.Set("X-Request-Id", reqID)
		c.Writer.Header().Set("X-Request-Id", reqID)
		c.Next()
	})
	s.engine.Use(gin.LoggerWithFormatter(func(p gin.LogFormatterParams) string {
		reqID := p.Request.Header.Get("X-Request-Id")
		return p.TimeStamp.Format("2006-01-02T15:04:05Z07:00") + " " + p.ClientIP + " " + p.Method + " " + p.Path + " " + strconv.Itoa(p.StatusCode) + " " + p.Latency.String() + " " + reqID + "\n"
	}))
	s.engine.Use(gin.Recovery())
	s.engine.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})
	s.engine.Use(s.authMiddleware())
	s.routesGin()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.engine
}

func (s *Server) routesGin() {
	s.engine.Static("/assets", s.cfg.AssetsDir)
	s.engine.POST("/api/login", func(c *gin.Context) { s.handleLogin(c.Writer, c.Request) })
	s.engine.GET("/api/specs", func(c *gin.Context) { s.handleSpecs(c.Writer, c.Request) })
	s.engine.POST("/api/specs", func(c *gin.Context) { s.handleUpdateSpecs(c.Writer, c.Request) })
	s.engine.POST("/api/upload", func(c *gin.Context) { s.handleUpload(c.Writer, c.Request) })
	s.engine.GET("/api/me", func(c *gin.Context) { s.handleMe(c.Writer, c.Request) })
	s.engine.POST("/api/tasks", func(c *gin.Context) { s.handleCreateTask(c.Writer, c.Request) })
	s.engine.GET("/api/tasks/:id", func(c *gin.Context) {
		r := c.Request.Clone(c.Request.Context())
		r.URL.Path = "/api/tasks/" + c.Param("id")
		s.handleGetTask(c.Writer, r)
	})
	s.engine.POST("/api/tasks/:id/background", func(c *gin.Context) {
		r := c.Request.Clone(c.Request.Context())
		r.URL.Path = "/api/tasks/" + c.Param("id") + "/background"
		s.handleGenerateBackground(c.Writer, r)
	})
	s.engine.POST("/api/tasks/:id/layout", func(c *gin.Context) {
		r := c.Request.Clone(c.Request.Context())
		r.URL.Path = "/api/tasks/" + c.Param("id") + "/layout"
		s.handleGenerateLayout(c.Writer, r)
	})
	s.engine.POST("/api/download/token", func(c *gin.Context) { s.handleDownloadToken(c.Writer, c.Request) })
	s.engine.GET("/api/download/file", func(c *gin.Context) { s.handleDownloadFile(c.Writer, c.Request) })
	s.engine.GET("/api/download/:id", func(c *gin.Context) {
		r := c.Request.Clone(c.Request.Context())
		r.URL.Path = "/api/download/" + c.Param("id")
		s.handleDownloadInfo(c.Writer, r)
	})
	s.engine.POST("/api/orders", func(c *gin.Context) { s.handleOrders(c.Writer, c.Request) })
	s.engine.GET("/api/orders", func(c *gin.Context) { s.handleOrders(c.Writer, c.Request) })
	s.engine.GET("/api/orders/:id", func(c *gin.Context) {
		r := c.Request.Clone(c.Request.Context())
		r.URL.Path = "/api/orders/" + c.Param("id")
		s.handleGetOrder(c.Writer, r)
	})
	s.engine.POST("/api/pay/wechat", func(c *gin.Context) { s.handlePayWechat(c.Writer, c.Request) })
	s.engine.POST("/api/pay/douyin", func(c *gin.Context) { s.handlePayDouyin(c.Writer, c.Request) })
	s.engine.POST("/api/pay/callback", func(c *gin.Context) { s.handlePayCallback(c.Writer, c.Request) })
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
	SpecCode          string   `json:"specCode"`
	SourceObjectKey   string   `json:"sourceObjectKey"`
	DefaultBackground string   `json:"defaultBackground"`
	AvailableColors   []string `json:"availableColors"`
	Colors            []string `json:"colors"`
	WidthPx           int      `json:"widthPx"`
	HeightPx          int      `json:"heightPx"`
	DPI               int      `json:"dpi"`
}

type generateBackgroundReq struct {
	Color  string `json:"color"`
	DPI    int    `json:"dpi"`
	Render int    `json:"render"`
	KB     int    `json:"kb"`
}

type generateLayoutReq struct {
	Color    string `json:"color"`
	WidthPx  int    `json:"widthPx"`
	HeightPx int    `json:"heightPx"`
	DPI      int    `json:"dpi"`
	KB       int    `json:"kb"`
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
	TaskID      string             `json:"taskId"`
	Items       []domain.OrderItem `json:"items"`
	City        string             `json:"city"`
	Remark      string             `json:"remark"`
	AmountCents int                `json:"amountCents"`
	Channel     string             `json:"channel"`
}

func (s *Server) handleSpecs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only GET accepted")
		return
	}
	if s.pg != nil {
		if items, err := s.pg.ListSpecs(); err == nil && len(items) > 0 {
			out := make([]Spec, 0, len(items))
			for _, it := range items {
				out = append(out, Spec{
					Code: it.Code, Name: it.Name, WidthPx: it.WidthPx, HeightPx: it.HeightPx, DPI: it.DPI, BgColors: it.BgColors,
				})
			}
			s.json(w, r, http.StatusOK, out)
			return
		}
	}
	s.json(w, r, http.StatusOK, s.defaultSpecs())
}

func (s *Server) defaultSpecs() []Spec {
	return []Spec{
		{Code: "passport", Name: "护照", WidthPx: 354, HeightPx: 472, DPI: 300, BgColors: []string{"white", "blue", "red"}},
		{Code: "cn_1inch", Name: "一寸", WidthPx: 295, HeightPx: 413, DPI: 300, BgColors: []string{"white", "blue", "red", "tint", "grey", "gradient", "dark_blue", "sky_blue"}},
		{Code: "cn_2inch", Name: "二寸", WidthPx: 413, HeightPx: 579, DPI: 300, BgColors: []string{"white", "blue", "red", "tint", "grey", "gradient", "dark_blue", "sky_blue"}},
		{Code: "cn_2inch_small", Name: "小二寸", WidthPx: 413, HeightPx: 531, DPI: 300, BgColors: []string{"white", "blue", "red", "tint", "grey", "gradient", "dark_blue", "sky_blue"}},
		{Code: "cn_1inch_large", Name: "大一寸", WidthPx: 390, HeightPx: 567, DPI: 300, BgColors: []string{"white", "blue", "red", "tint", "grey", "gradient", "dark_blue", "sky_blue"}},
	}
}

func (s *Server) handleUpdateSpecs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only POST accepted")
		return
	}
	if s.pg == nil {
		s.err(w, r, http.StatusNotImplemented, "NotImplemented", "PostgreSQL not configured")
		return
	}
	siteBg := []string{"blue", "white", "red", "tint", "grey", "gradient", "dark_blue", "sky_blue"}
	specs := []domain.SpecDef{
		{Code: "cn_1inch", Name: "一寸", WidthPx: 295, HeightPx: 413, DPI: 300, BgColors: siteBg},
		{Code: "cn_2inch", Name: "二寸", WidthPx: 413, HeightPx: 579, DPI: 300, BgColors: siteBg},
		{Code: "cn_2inch_small", Name: "小二寸", WidthPx: 413, HeightPx: 531, DPI: 300, BgColors: siteBg},
		{Code: "cn_1inch_large", Name: "大一寸", WidthPx: 390, HeightPx: 567, DPI: 300, BgColors: siteBg},
		{Code: "cn_social_security", Name: "社保证（300dpi，无回执）", WidthPx: 358, HeightPx: 441, DPI: 300, BgColors: []string{"white"}},
	}
	if err := s.pg.UpsertSpecs(specs); err != nil {
		s.err(w, r, http.StatusInternalServerError, "ServerError", "update specs failed")
		return
	}
	s.json(w, r, http.StatusOK, map[string]any{"updated": len(specs)})
}

type loginReq struct {
	Code string `json:"code"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only POST accepted")
		return
	}
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "invalid json")
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "code required")
		return
	}
	token, u, err := s.authSvc.Login(req.Code)
	if err != nil {
		s.err(w, r, http.StatusBadGateway, "WechatError", err.Error())
		return
	}
	s.json(w, r, http.StatusOK, map[string]any{"token": token, "userId": u.UserID, "openid": u.OpenID})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only GET accepted")
		return
	}
	authz := r.Header.Get("Authorization")
	if !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		s.err(w, r, http.StatusUnauthorized, "Unauthorized", "token required")
		return
	}
	tk := strings.TrimSpace(authz[7:])
	uid, oid, err := s.authSvc.Verify(tk)
	if err != nil || strings.TrimSpace(uid) == "" || strings.TrimSpace(oid) == "" {
		s.err(w, r, http.StatusUnauthorized, "Unauthorized", "token invalid")
		return
	}
	u, ok := s.authSvc.Repo.GetUserByOpenID(oid)
	if !ok || u == nil {
		s.json(w, r, http.StatusOK, map[string]any{
			"userId":   uid,
			"openid":   oid,
			"nickname": "",
			"avatar":   "",
		})
		return
	}
	s.json(w, r, http.StatusOK, u)
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
		if req.AmountCents <= 0 {
			s.err(w, r, http.StatusBadRequest, "BadRequest", "amountCents required")
			return
		}
		if len(req.Items) == 0 {
			s.err(w, r, http.StatusBadRequest, "BadRequest", "items required")
			return
		}
		for _, it := range req.Items {
			if strings.TrimSpace(it.Type) == "" || it.Qty <= 0 {
				s.err(w, r, http.StatusBadRequest, "BadRequest", "invalid items")
				return
			}
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
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "Idempotency-Key required")
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
	p, err := s.orderSvc.Pay(req.OrderID, channel, idempotencyKey)
	if err != nil {
		switch err.(type) {
		case usecase.ErrNotFound:
			s.err(w, r, http.StatusNotFound, "NotFound", "order not found")
		case usecase.ErrConflict:
			s.err(w, r, http.StatusConflict, "Conflict", err.Error())
		case usecase.ErrBadRequest:
			s.err(w, r, http.StatusBadRequest, "BadRequest", err.Error())
		default:
			s.err(w, r, http.StatusInternalServerError, "ServerError", "payment failed")
		}
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
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "Idempotency-Key required")
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
	if err := s.orderSvc.Callback(req.OrderID, strings.ToLower(req.Status)); err != nil {
		switch err.(type) {
		case usecase.ErrNotFound:
			s.err(w, r, http.StatusNotFound, "NotFound", "order not found")
		case usecase.ErrBadRequest:
			s.err(w, r, http.StatusBadRequest, "BadRequest", err.Error())
		default:
			s.err(w, r, http.StatusInternalServerError, "ServerError", "callback failed")
		}
		return
	}
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
	userID := "dev-user"
	authz := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		tk := strings.TrimSpace(authz[7:])
		if uid, _, err := s.authSvc.Verify(tk); err == nil && strings.TrimSpace(uid) != "" {
			userID = uid
		}
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
	if len(req.AvailableColors) == 0 {
		if len(req.Colors) != 0 {
			req.AvailableColors = req.Colors
		} else {
			req.AvailableColors = spec.BgColors
		}
	}
	t, _ := s.taskSvc.CreateTask(userID, orDefault(req.SpecCode, "passport"), req.SourceObjectKey, req.DefaultBackground, req.WidthPx, req.HeightPx, req.DPI, req.AvailableColors, colorHexOf)
	s.json(w, r, http.StatusOK, t)
}

func (s *Server) handleGenerateBackground(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only POST accepted")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "task id required")
		return
	}
	id := parts[0]
	var req generateBackgroundReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "invalid json")
		return
	}
	if strings.TrimSpace(req.Color) == "" {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "color required")
		return
	}
	t, ok := s.taskSvc.Repo.Get(id)
	if !ok {
		s.err(w, r, http.StatusNotFound, "NotFound", "task not found")
		return
	}
	dpi := req.DPI
	if dpi == 0 {
		dpi = t.Spec.DPI
	}
	url, err := s.taskSvc.GenerateBackground(id, req.Color, dpi, colorHexOf)
	if err != nil {
		if _, ok := err.(usecase.ErrNotFound); ok {
			s.err(w, r, http.StatusNotFound, "NotFound", err.Error())
			return
		}
		s.err(w, r, http.StatusInternalServerError, "ServerError", "generate background failed")
		return
	}
	s.json(w, r, http.StatusOK, map[string]any{
		"taskId": id,
		"color":  req.Color,
		"url":    url,
		"status": "done",
	})
}

func (s *Server) handleGenerateLayout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only POST accepted")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "task id required")
		return
	}
	id := parts[0]
	var req generateLayoutReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "invalid json")
		return
	}
	if strings.TrimSpace(req.Color) == "" {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "color required")
		return
	}
	t, ok := s.taskSvc.Repo.Get(id)
	if !ok {
		s.err(w, r, http.StatusNotFound, "NotFound", "task not found")
		return
	}
	width := req.WidthPx
	height := req.HeightPx
	dpi := req.DPI
	if width == 0 || height == 0 || dpi == 0 {
		sp := s.findSpec(orDefault(t.SpecCode, "passport"))
		if width == 0 {
			width = sp.WidthPx
		}
		if height == 0 {
			height = sp.HeightPx
		}
		if dpi == 0 {
			dpi = sp.DPI
		}
	}
	url, err := s.taskSvc.GenerateLayout(id, req.Color, width, height, dpi, req.KB, colorHexOf)
	if err != nil {
		if _, ok := err.(usecase.ErrNotFound); ok {
			s.err(w, r, http.StatusNotFound, "NotFound", err.Error())
			return
		}
		s.err(w, r, http.StatusInternalServerError, "ServerError", "generate layout failed")
		return
	}
	s.json(w, r, http.StatusOK, map[string]any{
		"taskId": id,
		"layout": "6inch",
		"url":    url,
		"status": "done",
	})
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

type downloadTokenReq struct {
	TaskID     string `json:"taskId"`
	TTLSeconds int    `json:"ttlSeconds"`
}

func (s *Server) handleDownloadToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only POST accepted")
		return
	}
	authz := r.Header.Get("Authorization")
	if !strings.HasPrefix(strings.ToLower(authz), "bearer ") {
		s.err(w, r, http.StatusUnauthorized, "Unauthorized", "token required")
		return
	}
	tk := strings.TrimSpace(authz[7:])
	uid, _, err := s.authSvc.Verify(tk)
	if err != nil || strings.TrimSpace(uid) == "" {
		s.err(w, r, http.StatusUnauthorized, "Unauthorized", "token invalid")
		return
	}
	var req downloadTokenReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "invalid json")
		return
	}
	dt, err := s.downloadSvc.CreateToken(req.TaskID, uid, req.TTLSeconds)
	if err != nil {
		if _, ok := err.(usecase.ErrNotFound); ok {
			s.err(w, r, http.StatusNotFound, "NotFound", err.Error())
			return
		}
		if _, ok := err.(usecase.ErrConflict); ok {
			s.err(w, r, http.StatusConflict, "Conflict", err.Error())
			return
		}
		if _, ok := err.(usecase.ErrBadRequest); ok {
			s.err(w, r, http.StatusBadRequest, "BadRequest", err.Error())
			return
		}
		s.err(w, r, http.StatusInternalServerError, "ServerError", "create token failed")
		return
	}
	s.json(w, r, http.StatusOK, map[string]any{
		"token":     dt.Token,
		"expiresAt": dt.ExpiresAt,
	})
}

func (s *Server) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.err(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "only GET accepted")
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	dt, err := s.downloadSvc.UseToken(token)
	if err != nil {
		if _, ok := err.(usecase.ErrNotFound); ok {
			s.err(w, r, http.StatusNotFound, "NotFound", err.Error())
			return
		}
		if _, ok := err.(usecase.ErrConflict); ok {
			s.err(w, r, http.StatusConflict, "Conflict", err.Error())
			return
		}
		if _, ok := err.(usecase.ErrBadRequest); ok {
			s.err(w, r, http.StatusBadRequest, "BadRequest", err.Error())
			return
		}
		s.err(w, r, http.StatusInternalServerError, "ServerError", "download failed")
		return
	}
	t, ok := s.taskSvc.Repo.Get(dt.TaskID)
	if !ok {
		s.err(w, r, http.StatusNotFound, "NotFound", "task not found")
		return
	}
	if t.Status != domain.StatusDone {
		s.err(w, r, http.StatusBadRequest, "BadRequest", "task not ready")
		return
	}
	type entry struct {
		name string
		path string
	}
	entries := make([]entry, 0, 8)
	if name, path, ok := s.assetEntry(t.BaselineUrl); ok {
		entries = append(entries, entry{name: name, path: path})
	}
	for _, url := range t.ProcessedUrls {
		if name, path, ok := s.assetEntry(url); ok {
			entries = append(entries, entry{name: name, path: path})
		}
	}
	for _, url := range t.LayoutUrls {
		if name, path, ok := s.assetEntry(url); ok {
			entries = append(entries, entry{name: name, path: path})
		}
	}
	if len(entries) == 0 {
		s.err(w, r, http.StatusNotFound, "NotFound", "assets not found")
		return
	}
	for _, e := range entries {
		if _, err := os.Stat(e.path); err != nil {
			s.err(w, r, http.StatusNotFound, "NotFound", "asset not found")
			return
		}
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"task_"+t.ID+".zip\"")
	zw := zip.NewWriter(w)
	for _, e := range entries {
		f, err := os.Open(e.path)
		if err != nil {
			break
		}
		wr, err := zw.Create(e.name)
		if err == nil {
			_, _ = io.Copy(wr, f)
		}
		_ = f.Close()
	}
	_ = zw.Close()
}

func validImageName(name string) bool {
	n := strings.ToLower(name)
	return strings.HasSuffix(n, ".jpg") || strings.HasSuffix(n, ".jpeg") || strings.HasSuffix(n, ".png")
}

func (s *Server) assetEntry(url string) (string, string, bool) {
	url = strings.TrimSpace(url)
	if url == "" {
		return "", "", false
	}
	idx := strings.Index(url, "/assets/")
	if idx < 0 {
		return "", "", false
	}
	rel := strings.TrimPrefix(url[idx:], "/assets/")
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", "", false
	}
	path := filepath.Join(s.cfg.AssetsDir, filepath.FromSlash(rel))
	return rel, path, true
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
	reqID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
	if reqID == "" {
		reqID = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	if reqID != "" {
		w.Header().Set("X-Request-Id", reqID)
	}
	log.Printf("error %s %s %d %s %s", r.Method, r.URL.Path, status, reqID, msg)
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

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/assets") || p == "/api/login" || strings.HasPrefix(p, "/api/download/file") {
			c.Next()
			return
		}
		authz := c.GetHeader("Authorization")
		if strings.HasPrefix(strings.ToLower(authz), "bearer ") {
			tk := strings.TrimSpace(authz[7:])
			if uid, _, err := s.authSvc.Verify(tk); err == nil && strings.TrimSpace(uid) != "" {
				c.Next()
				return
			}
		}
		s.err(c.Writer, c.Request, http.StatusUnauthorized, "Unauthorized", "token required")
		c.Abort()
	}
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
func (algoAdapter) AddBackgroundFile(baseURL string, rgbaPNG []byte, colorHex string, dpi int) (algo.AddBackgroundResp, error) {
	return algo.AddBackgroundFile(baseURL, rgbaPNG, colorHex, dpi)
}
func (algoAdapter) GenerateLayoutPhotosFile(baseURL string, rgbImage []byte, height, width, dpi, kb int) (algo.LayoutResp, error) {
	return algo.GenerateLayoutPhotosFile(baseURL, rgbImage, height, width, dpi, kb)
}

type pgOrderRepo struct{ pg *repo.PostgresRepo }

func (p *pgOrderRepo) Put(o *domain.Order) error           { return p.pg.PutOrder(o) }
func (p *pgOrderRepo) Get(id string) (*domain.Order, bool) { return p.pg.GetOrder(id) }
func (p *pgOrderRepo) List(page, pageSize int) ([]domain.Order, int) {
	return p.pg.ListOrders(page, pageSize)
}
