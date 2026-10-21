package routers

import (
	"net/http"

	"app-api/pkg/auth"
	"app-api/pkg/chats"
	"app-api/pkg/diet"
	"app-api/pkg/family"
	"app-api/pkg/memories"
	"app-api/pkg/notes"
	"app-api/pkg/reminders"
	"app-api/pkg/speech"
	"app-api/pkg/weather"
	"common/middleware/httpx"
	"common/middleware/routers"
	"common/viewer"
	"service/identity"
	"service/wire"

	"github.com/gin-gonic/gin"
)

func New(services *wire.Services) http.Handler {
	engine := gin.New()
	engine.Use(gin.Recovery())
	_ = engine.SetTrustedProxies(nil)
	engine.GET("/healthz", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// /healthz 留在根上给启动脚本探活；接口带上 server.context_path 前缀
	api := engine.Group(httpx.ContextPath() + "/api")
	auth.New(services.Identity, identity.RealmApp).RegisterHandler(api)
	protected := api.Group("", httpx.RequireUser(), withViewer())
	for _, handler := range []routers.Handler{
		chats.New(services.Chat, services.Bots),
		memories.New(services.Memory),
		notes.New(services.Intake, "paste"),
		family.New(services.Family, services.Persona),
		reminders.New(services.Reminder),
		speech.New(services.LLM),
		diet.New(services.Diet),
		weather.New(services.Weather),
	} {
		handler.RegisterHandler(protected)
	}
	return engine
}

// withViewer 把登录账号放进请求的 context：记忆、原始记录、提醒只查得到全家可见的和自己的。
// 奶奶的账号锁定为只能私有。
func withViewer() gin.HandlerFunc {
	return func(c *gin.Context) {
		principal := httpx.MustUser(c)
		ctx := viewer.With(c.Request.Context(), viewer.Viewer{Account: principal.AccountKey, Locked: principal.Role == identity.RoleElder})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
