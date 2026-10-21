package routers

import (
	"net/http"

	"common/middleware/httpx"
	"common/middleware/routers"
	"common/viewer"
	"manager-api/pkg/accounts"
	"manager-api/pkg/auth"
	"manager-api/pkg/bots"
	"manager-api/pkg/conversations"
	"manager-api/pkg/diet"
	"manager-api/pkg/family"
	"manager-api/pkg/insights"
	"manager-api/pkg/memories"
	"manager-api/pkg/models"
	"manager-api/pkg/notes"
	"manager-api/pkg/personas"
	"manager-api/pkg/reminders"
	"manager-api/pkg/settings"
	"manager-api/pkg/skills"
	"manager-api/pkg/speech"
	"manager-api/pkg/weather"
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
	auth.New(services.Identity, identity.RealmManager).RegisterHandler(api)

	// 管理端是我在维护：按系统账号（我自己）的身份看记忆、原始记录、提醒，看不到奶奶的私有记录（common/viewer）
	protected := api.Group("", httpx.RequireUser(), func(c *gin.Context) {
		c.Request = c.Request.WithContext(viewer.AsSystem(c.Request.Context()))
		c.Next()
	})
	for _, handler := range []routers.Handler{
		accounts.New(services.Identity),
		family.New(services.Family),
		memories.New(services.Memory),
		notes.New(services.Intake, "manager"),
		personas.New(services.Persona),
		bots.New(services.Bots),
		skills.New(services.Skills),
		settings.New(services.Settings, services.Chat),
		insights.New(services.Insight),
		models.New(services.LLM, services.ReindexInBackground),
		speech.New(services.LLM),
		conversations.New(services.Chat),
		reminders.New(services.Reminder),
		diet.New(services.Diet),
		weather.New(services.Weather),
	} {
		handler.RegisterHandler(protected)
	}
	return engine
}
