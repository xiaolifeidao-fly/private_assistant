package main

import (
	"context"
	"log"
	"strconv"
	"time"

	"common/middleware/db"
	"common/middleware/httpx"
	"manager-api/routers"
	"migrations"
	"service/identity"
	"service/wire"

	"github.com/gin-gonic/gin"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	database := httpx.Boot("manager-api")
	if err := db.Migrate(database, migrations.FS()); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	options := wire.Options{
		Identity: identity.Config{
			ManagerTTL: time.Duration(httpx.PropertyInt("auth.manager_ttl_seconds", 7*24*3600)) * time.Second,
		},
		AnthropicAPIKey:  httpx.Property("llm.anthropic.api_key"),
		AnthropicBaseURL: httpx.Property("llm.anthropic.base_url"),
		EmbeddingBaseURL: httpx.Property("llm.embedding.base_url"),
		EmbeddingAPIKey:  httpx.Property("llm.embedding.api_key"),
		VAPIDPublicKey:   httpx.Property("push.vapid_public_key"),
		VAPIDPrivateKey:  httpx.Property("push.vapid_private_key"),
		VAPIDSubject:     httpx.Property("push.vapid_subject"),
	}
	options.LoadStorage(httpx.Property)
	if value, err := strconv.ParseFloat(httpx.Property("extract.auto_confirm"), 64); err == nil {
		options.AutoConfirm = value
	}
	services := wire.Build(database, options)
	services.Seed(options)
	if httpx.Property("worker.enabled") != "false" {
		services.StartWorkers(context.Background())
	}

	// 首个管理员：管理端还没有任何账号时，用配置里的初始账号密码创建。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	created, err := services.Identity.EnsureAdmin(ctx, identity.RealmManager,
		httpx.Property("bootstrap.admin_username"), httpx.Property("bootstrap.admin_password"))
	cancel()
	if err != nil {
		log.Fatalf("创建初始管理员失败: %v", err)
	}
	if created {
		log.Printf("已创建初始管理员 %s，请登录后尽快修改密码", httpx.Property("bootstrap.admin_username"))
	}

	httpx.SetAuthenticator(authenticator{services.Identity})
	address := httpx.Property("server.address")
	if address == "" {
		address = ":10013"
	}
	httpx.Serve(routers.New(services), address)
}

// authenticator 只认管理端（realm=manager）的令牌。
type authenticator struct{ identity *identity.Service }

func (a authenticator) AuthenticateToken(ctx context.Context, token string) (httpx.Principal, error) {
	account, err := a.identity.Authenticate(ctx, identity.RealmManager, token)
	if err != nil {
		return httpx.Principal{}, err
	}
	return httpx.Principal{
		AccountKey: account.AccountKey, Username: account.Username, DisplayName: account.DisplayName,
		Realm: account.Realm, Role: account.Role, FamilyKey: account.FamilyKey,
	}, nil
}
