package routers

import (
	"net/http"

	"app-api/pkg/auth"
	"app-api/pkg/chats"
	"app-api/pkg/family"
	"app-api/pkg/memories"
	"app-api/pkg/notes"
	"app-api/pkg/reminders"
	"app-api/pkg/speech"
	"common/middleware/httpx"
	"common/middleware/routers"
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
	protected := api.Group("", httpx.RequireUser())
	for _, handler := range []routers.Handler{
		chats.New(services.Chat, services.Bots),
		memories.New(services.Memory),
		notes.New(services.Intake, "paste"),
		family.New(services.Family, services.Persona),
		reminders.New(services.Reminder),
		speech.New(services.LLM),
	} {
		handler.RegisterHandler(protected)
	}
	return engine
}
