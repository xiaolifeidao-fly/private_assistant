package main

import (
	"context"
	"log"
	"strconv"
	"time"

	"app-api/routers"
	"common/middleware/db"
	"common/middleware/httpx"
	"migrations"
	"service/identity"
	"service/wire"

	"github.com/gin-gonic/gin"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	database := httpx.Boot("app-api")
	if err := db.Migrate(database, migrations.FS()); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}
	options := wire.Options{
		Identity: identity.Config{
			// 手机主屏 PWA 的存储和 Safari 隔离，令牌过期就得重新登录，所以给长一点。
			AppTTL: time.Duration(httpx.PropertyInt("auth.app_ttl_seconds", 90*24*3600)) * time.Second,
		},
		VAPIDPublicKey:  httpx.Property("push.vapid_public_key"),
		VAPIDPrivateKey: httpx.Property("push.vapid_private_key"),
		VAPIDSubject:    httpx.Property("push.vapid_subject"),
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	created, err := services.Identity.EnsureAdmin(ctx, identity.RealmApp,
		httpx.Property("bootstrap.app_username"), httpx.Property("bootstrap.app_password"))
	cancel()
	if err != nil {
		log.Fatalf("创建初始 App 账号失败: %v", err)
	}
	if created {
		log.Printf("已创建 App 账号 %s", httpx.Property("bootstrap.app_username"))
	}

	httpx.SetAuthenticator(authenticator{services.Identity})
	address := httpx.Property("server.address")
	if address == "" {
		address = ":10012"
	}
	httpx.Serve(routers.New(services), address)
}

// authenticator 只认 App（realm=app）的令牌。
type authenticator struct{ identity *identity.Service }

func (a authenticator) AuthenticateToken(ctx context.Context, token string) (httpx.Principal, error) {
	account, err := a.identity.Authenticate(ctx, identity.RealmApp, token)
	if err != nil {
		return httpx.Principal{}, err
	}
	return httpx.Principal{
		AccountKey: account.AccountKey, Username: account.Username, DisplayName: account.DisplayName,
		Realm: account.Realm, Role: account.Role, FamilyKey: account.FamilyKey, PersonKey: account.PersonKey,
	}, nil
}
