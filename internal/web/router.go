package web

import (
	"errors"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"mgfilebox/internal/config"
	"mgfilebox/internal/models"
	"mgfilebox/internal/repository"
	"mgfilebox/internal/service"

	"github.com/gin-gonic/gin"
)

const adminCookieName = "mgbox_admin"

type Server struct {
	engine       *gin.Engine
	svc          *service.Service
	cfg          *config.Config
	loginLimiter *loginLimiter
}

type viewData struct {
	Title         string
	ActiveNav     string
	Now           time.Time
	Error         string
	Share         *models.Share
	Locked        bool
	Expired       bool
	Deleted       bool
	Shares        []models.ShareSummary
	DeletedShares []models.ShareSummary
	DownloadCount int64
}

type loginAttempt struct {
	failures int
	lockedAt time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
	now      func() time.Time
}

const (
	maxLoginFailures = 5
	loginLockout     = 15 * time.Minute
)

func NewServer(cfg *config.Config, svc *service.Service) (*Server, error) {
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery())
	engine.MaxMultipartMemory = cfg.MaxUploadSize

	tmpl, err := template.New("").Funcs(template.FuncMap{
		"formatTime": func(value time.Time) string {
			return value.Local().Format("2006-01-02 15:04")
		},
		"formatExpiry": func(value time.Time) string {
			if models.IsNeverExpiresTime(value) {
				return "永不过期"
			}
			return value.Local().Format("2006-01-02 15:04")
		},
		"formatSize": formatFileSize,
		"formatDeletedTime": func(value *time.Time) string {
			if value == nil {
				return "-"
			}
			return value.Local().Format("2006-01-02 15:04")
		},
	}).ParseGlob(filepath.Join("web", "templates", "*.html"))
	if err != nil {
		return nil, err
	}

	engine.SetHTMLTemplate(tmpl)
	engine.Static("/static", filepath.Join(".", "web", "static"))

	server := &Server{
		engine:       engine,
		svc:          svc,
		cfg:          cfg,
		loginLimiter: newLoginLimiter(),
	}
	server.registerRoutes()
	return server, nil
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{
		attempts: make(map[string]loginAttempt),
		now:      time.Now,
	}
}

func (l *loginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	attempt, ok := l.attempts[key]
	if !ok || attempt.lockedAt.IsZero() {
		return true
	}
	if l.now().Sub(attempt.lockedAt) >= loginLockout {
		delete(l.attempts, key)
		return true
	}
	return false
}

func (l *loginLimiter) recordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	attempt := l.attempts[key]
	attempt.failures++
	if attempt.failures >= maxLoginFailures {
		attempt.lockedAt = l.now()
	}
	l.attempts[key] = attempt
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.attempts, key)
}

func formatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	divisor := int64(unit)
	units := []string{"KB", "MB", "GB", "TB"}
	for _, suffix := range units {
		if size < divisor*unit || suffix == units[len(units)-1] {
			return fmt.Sprintf("%.1f %s", float64(size)/float64(divisor), suffix)
		}
		divisor *= unit
	}
	return fmt.Sprintf("%d B", size)
}

func (s *Server) Engine() *gin.Engine {
	return s.engine
}

func (s *Server) registerRoutes() {
	s.engine.GET("/login", s.handleLoginPage)
	s.engine.POST("/api/auth/login", s.handleLogin)
	s.engine.GET("/s/:id", s.handleSharePage)
	s.engine.POST("/s/:id/unlock", s.handleUnlockShare)
	s.engine.GET("/s/:id/download", s.handleDownload)

	admin := s.engine.Group("/")
	admin.Use(s.requireAdmin())
	admin.GET("/", func(c *gin.Context) { c.Redirect(http.StatusFound, "/upload") })
	admin.GET("/upload", s.handleUploadPage)
	admin.GET("/logout", s.handleLogout)
	admin.GET("/admin/shares", s.handleAdminPage)
	admin.POST("/api/shares/file", s.handleCreateFileShare)
	admin.POST("/api/admin/shares/:id/delete", s.handleDeleteShare)
	admin.POST("/api/admin/shares/:id/purge", s.handlePurgeDeletedShare)
}

func (s *Server) requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(adminCookieName)
		if err != nil || token == "" || s.svc.ValidateAdminSession(c.Request.Context(), token) != nil {
			c.SetCookie(adminCookieName, "", -1, "/", "", shouldUseSecureCookie(c), true)
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) handleLoginPage(c *gin.Context) {
	token, err := c.Cookie(adminCookieName)
	if err == nil && token != "" && s.svc.ValidateAdminSession(c.Request.Context(), token) == nil {
		c.Redirect(http.StatusFound, "/upload")
		return
	}

	c.HTML(http.StatusOK, "login.html", viewData{
		Title: "管理员登录",
		Now:   time.Now(),
	})
}

func (s *Server) handleUploadPage(c *gin.Context) {
	c.HTML(http.StatusOK, "upload.html", viewData{
		Title:     "上传分享",
		ActiveNav: "upload",
		Now:       time.Now(),
	})
}

func (s *Server) handleAdminPage(c *gin.Context) {
	shares, err := s.svc.ListShares(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "加载分享列表失败")
		return
	}
	deletedShares, err := s.svc.ListDeletedShares(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "加载已删除列表失败")
		return
	}

	c.HTML(http.StatusOK, "admin.html", viewData{
		Title:         "分享管理",
		ActiveNav:     "admin",
		Now:           time.Now(),
		Shares:        shares,
		DeletedShares: deletedShares,
	})
}

func (s *Server) handleSharePage(c *gin.Context) {
	share, err := s.svc.GetShare(c.Request.Context(), c.Param("id"))
	if err != nil {
		s.renderShareStatus(c, nil, false, false, true)
		return
	}

	if share.IsDeleted() {
		s.renderShareStatus(c, share, false, false, true)
		return
	}
	if share.IsExpired(time.Now()) {
		s.renderShareStatus(c, share, false, true, false)
		return
	}

	pickcodeError := ""
	if pickcode := strings.TrimSpace(c.Query("pickcode")); pickcode != "" && share.HasPassword() {
		value, verifyErr := s.svc.VerifySharePassword(share, pickcode)
		if verifyErr == nil {
			c.SetCookie(unlockCookieName(share.ID), value, unlockCookieMaxAge(share.ExpiresAt), "/s/"+share.ID, "", shouldUseSecureCookie(c), true)
			c.Redirect(http.StatusFound, "/s/"+share.ID)
			return
		}
		pickcodeError = verifyErr.Error()
	}

	unlockCookie, _ := c.Cookie(unlockCookieName(share.ID))
	locked := !s.svc.CanAccessShare(share, unlockCookie)
	downloadCount := int64(0)
	if !locked {
		count, countErr := s.svc.CountSuccessfulDownloads(c.Request.Context(), share.ID)
		if countErr == nil {
			downloadCount = count
		}
	}

	s.svc.RecordAccess(c.Request.Context(), share.ID, "view", clientIP(c), c.Request.UserAgent(), http.StatusOK)
	c.HTML(http.StatusOK, "share.html", viewData{
		Title:         share.Title,
		Now:           time.Now(),
		Share:         share,
		Locked:        locked,
		Error:         pickcodeError,
		DownloadCount: downloadCount,
	})
}

func (s *Server) handleUnlockShare(c *gin.Context) {
	share, err := s.svc.GetShare(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.Redirect(http.StatusFound, "/s/"+c.Param("id"))
		return
	}

	value, err := s.svc.VerifySharePassword(share, c.PostForm("password"))
	if err != nil {
		c.HTML(http.StatusUnauthorized, "share.html", viewData{
			Title:  share.Title,
			Now:    time.Now(),
			Share:  share,
			Locked: true,
			Error:  err.Error(),
		})
		return
	}

	c.SetCookie(unlockCookieName(share.ID), value, unlockCookieMaxAge(share.ExpiresAt), "/s/"+share.ID, "", shouldUseSecureCookie(c), true)
	c.Redirect(http.StatusFound, "/s/"+share.ID)
}

func (s *Server) handleDownload(c *gin.Context) {
	share, err := s.svc.GetShare(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.String(http.StatusNotFound, "文件不存在")
		return
	}
	if share.IsDeleted() || share.IsExpired(time.Now()) {
		c.String(http.StatusGone, "分享已失效")
		return
	}

	unlockCookie, _ := c.Cookie(unlockCookieName(share.ID))
	if !s.svc.CanAccessShare(share, unlockCookie) {
		c.String(http.StatusForbidden, "需要先解锁分享")
		return
	}

	var target *models.ShareFile
	if fileID := c.Query("file"); fileID != "" {
		target, err = s.svc.GetShareFile(c.Request.Context(), share.ID, fileID)
		if err != nil {
			c.String(http.StatusNotFound, "文件不存在")
			return
		}
	} else if len(share.Files) > 0 {
		target = &share.Files[0]
	}
	if target == nil {
		c.String(http.StatusNotFound, "文件不存在")
		return
	}

	s.svc.RecordAccess(c.Request.Context(), share.ID, "download", clientIP(c), c.Request.UserAgent(), http.StatusOK)
	c.FileAttachment(target.StoragePath, target.FileName)
}

func (s *Server) handleLogin(c *gin.Context) {
	limiterKey := clientIP(c)
	if !s.loginLimiter.allow(limiterKey) {
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": "登录失败次数过多，请 15 分钟后再试"})
		return
	}

	password := strings.TrimSpace(c.PostForm("password"))
	token, err := s.svc.Login(c.Request.Context(), password)
	if err != nil {
		s.loginLimiter.recordFailure(limiterKey)
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": err.Error()})
		return
	}

	s.loginLimiter.reset(limiterKey)
	c.SetCookie(adminCookieName, token, int(s.cfg.SessionTTL.Seconds()), "/", "", shouldUseSecureCookie(c), true)
	c.JSON(http.StatusOK, gin.H{"success": true, "redirect": "/upload"})
}

func (s *Server) handleLogout(c *gin.Context) {
	if token, err := c.Cookie(adminCookieName); err == nil && token != "" {
		_ = s.svc.Logout(c.Request.Context(), token)
	}
	c.SetCookie(adminCookieName, "", -1, "/", "", shouldUseSecureCookie(c), true)
	c.Redirect(http.StatusFound, "/login")
}

func (s *Server) handleCreateFileShare(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "请选择文件"})
		return
	}
	fileHeaders := form.File["files"]
	if len(fileHeaders) == 0 {
		fileHeaders = form.File["file"]
	}
	input, err := parseCommonInput(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	summary, err := s.svc.CreateFileShare(c.Request.Context(), input, fileHeaders)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

func (s *Server) handleDeleteShare(c *gin.Context) {
	if err := s.svc.DeleteShare(c.Request.Context(), c.Param("id")); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, repository.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"success": false, "message": "删除失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handlePurgeDeletedShare(c *gin.Context) {
	if err := s.svc.PurgeDeletedShare(c.Request.Context(), c.Param("id")); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, repository.ErrNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"success": false, "message": "清除记录失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) renderShareStatus(c *gin.Context, share *models.Share, locked, expired, deleted bool) {
	title := "分享不存在"
	if share != nil {
		title = share.Title
	}
	c.HTML(http.StatusOK, "share.html", viewData{
		Title:   title,
		Now:     time.Now(),
		Share:   share,
		Locked:  locked,
		Expired: expired,
		Deleted: deleted,
	})
}

func parseCommonInput(c *gin.Context) (models.ShareInput, error) {
	expiresHours, err := strconv.Atoi(c.PostForm("expires_hours"))
	if err != nil {
		return models.ShareInput{}, errors.New("过期时间格式错误")
	}

	return models.ShareInput{
		Title:          c.PostForm("title"),
		AccessPassword: c.PostForm("access_password"),
		ExpiresHours:   expiresHours,
	}, nil
}

func unlockCookieMaxAge(expiresAt time.Time) int {
	if models.IsNeverExpiresTime(expiresAt) {
		return math.MaxInt32
	}

	seconds := int(time.Until(expiresAt).Seconds())
	if seconds < 1 {
		return 1
	}
	if seconds > math.MaxInt32 {
		return math.MaxInt32
	}
	return seconds
}

func unlockCookieName(shareID string) string {
	return "mgbox_unlock_" + shareID
}

func clientIP(c *gin.Context) string {
	if value := c.ClientIP(); value != "" {
		return value
	}
	return "unknown"
}

func shouldUseSecureCookie(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}
