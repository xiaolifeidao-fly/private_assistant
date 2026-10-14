// Package httpx 是两个 API 进程共用的启动、响应和鉴权中间件。
//
// 领域包不读配置、不碰 gin；装配层（*-api/main.go）用 Property 取配置，
// 构造好领域服务后注入。
package httpx

import (
	"bufio"
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"common/middleware/db"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var config map[string]string

// Property 读取 configs/application.properties 里的一项；环境变量
// PA_<KEY>（点换成下划线、全大写）优先，方便容器里覆盖。必须在 Boot 之后调用。
func Property(key string) string {
	envKey := "PA_" + strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(key))
	if value, ok := os.LookupEnv(envKey); ok {
		return strings.TrimSpace(value)
	}
	return config[key]
}

// ContextPath 是接口的访问前缀（server.context_path），接口挂在 ContextPath()+"/api" 下。
// 统一成「/开头、不带结尾 /」的形式，例如 "/assistant"；没配时是空串，接口就在 /api。
func ContextPath() string {
	path := strings.Trim(Property("server.context_path"), "/")
	if path == "" {
		return ""
	}
	return "/" + path
}

// PropertyInt 读整数配置，缺省或非法时返回 fallback。
func PropertyInt(key string, fallback int) int {
	value, err := strconv.Atoi(Property(key))
	if err != nil {
		return fallback
	}
	return value
}

// Boot 读配置并打开 MySQL 连接池。配置缺失直接 panic —— 起一个连不上库的进程没有意义。
func Boot(name string) *gorm.DB {
	values, err := properties("configs/application.properties")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("读取 configs/application.properties: %v", err)
	}
	if values == nil {
		values = map[string]string{}
	}
	config = values
	dsn := Property("sqlconn")
	if dsn == "" {
		log.Fatalf("%s: 缺少 sqlconn 配置（configs/application.properties 或环境变量 PA_SQLCONN）", name)
	}
	database, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logger.Error),
	})
	if err != nil {
		log.Fatalf("%s: 连接数据库失败: %v", name, err)
	}
	pool, err := database.DB()
	if err != nil {
		log.Fatalf("%s: %v", name, err)
	}
	pool.SetMaxOpenConns(30)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(30 * time.Minute)
	db.SetDatabase(database)
	return database
}

// Serve 启动 HTTP 服务并在 SIGINT/SIGTERM 时优雅退出。
// WriteTimeout 不设：对话接口是 SSE 长连接，一次回答可能持续几分钟。
func Serve(handler http.Handler, address string) {
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("listening on %s", address)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

// Principal 是一次请求的登录身份。
type Principal struct {
	AccountKey  string `json:"accountKey"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Realm       string `json:"realm"` // manager / app
	Role        string `json:"role"`
	FamilyKey   string `json:"familyKey"`
}

// Authenticator 由 identity 领域实现，进程启动时注入。
// 每个进程只认自己那一套令牌（manager-api 认 manager，app-api 认 app），
// realm 由注入的实现自己固定，不从请求里取。
type Authenticator interface {
	AuthenticateToken(ctx context.Context, token string) (Principal, error)
}

var (
	authenticatorMu sync.RWMutex
	authenticator   Authenticator
)

func SetAuthenticator(value Authenticator) {
	authenticatorMu.Lock()
	defer authenticatorMu.Unlock()
	authenticator = value
}

const principalKey = "httpx.principal"

// RequireUser 要求请求带有效令牌（头 Authorization: Bearer xxx 或 token: xxx）。
func RequireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := Token(c)
		if token == "" {
			Unauthorized(c)
			return
		}
		authenticatorMu.RLock()
		value := authenticator
		authenticatorMu.RUnlock()
		if value == nil {
			Fail(c, "认证服务尚未初始化")
			c.Abort()
			return
		}
		principal, err := value.AuthenticateToken(c.Request.Context(), token)
		if err != nil {
			Unauthorized(c)
			return
		}
		c.Set(principalKey, principal)
		c.Next()
	}
}

// RequireAdmin 在 RequireUser 之后用：只放行 role=admin。
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if principal, ok := CurrentUser(c); !ok || principal.Role != "admin" {
			Fail(c, "需要管理员权限")
			c.Abort()
			return
		}
		c.Next()
	}
}

// Unauthorized 统一返回 code=401，前端据此清令牌回登录页。
func Unauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusOK, gin.H{"success": false, "code": 401, "data": nil, "message": "请求失败", "error": "not login"})
}

func Token(c *gin.Context) string {
	if token := strings.TrimSpace(c.GetHeader("token")); token != "" {
		return token
	}
	return strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
}

func CurrentUser(c *gin.Context) (Principal, bool) {
	value, ok := c.Get(principalKey)
	if !ok {
		return Principal{}, false
	}
	principal, ok := value.(Principal)
	return principal, ok
}

// MustUser 只在挂了 RequireUser 的路由里用。
func MustUser(c *gin.Context) Principal {
	principal, _ := CurrentUser(c)
	return principal
}

// ClientIP 取 X-Forwarded-For 第一跳。可伪造，只用于限流和留痕，不参与鉴权。
func ClientIP(c *gin.Context) string {
	forwarded, _, _ := strings.Cut(c.GetHeader("X-Forwarded-For"), ",")
	if value := strings.TrimSpace(forwarded); value != "" {
		return value
	}
	if value := strings.TrimSpace(c.GetHeader("X-Real-IP")); value != "" {
		return value
	}
	return c.ClientIP()
}

// JSON 统一响应：成功 {success:true,code:0,data}，失败 {success:false,code:-1,error}。
// 失败也是 HTTP 200，前端看 success。
func JSON(c *gin.Context, data any, err error) {
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "code": -1, "data": nil, "message": "请求失败", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "code": 0, "data": data, "message": "请求成功", "error": nil})
}

func Fail(c *gin.Context, message string) {
	c.JSON(http.StatusOK, gin.H{"success": false, "code": -1, "data": nil, "message": "请求失败", "error": message})
}

// Bind 绑定 JSON 请求体，失败时已写好响应，调用方直接 return。
func Bind(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		Fail(c, "参数错误: "+err.Error())
		return false
	}
	return true
}

// QueryInt 读整数查询参数。
func QueryInt(c *gin.Context, key string, fallback int) int {
	value, err := strconv.Atoi(c.Query(key))
	if err != nil {
		return fallback
	}
	return value
}

func properties(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return values, scanner.Err()
}
