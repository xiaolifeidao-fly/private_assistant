package routers

import (
	"net/http"

	"common/middleware/httpx"
	"common/middleware/routers"
	"manager-api/pkg/accounts"
	"manager-api/pkg/auth"
	"manager-api/pkg/bots"
	"manager-api/pkg/conversations"
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
	"service/identity"
	"service/wire"

	"github.com/gin-gonic/gin"
)

func New(services *wire.Services) http.Handler {
	engine := gin.New()
	engine.Use(gin.Recovery())
	_ = engine.SetTrustedProxies(nil)
	engine.GET("/healthz", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	api := engine.Group("/api")
	auth.New(services.Identity, identity.RealmManager).RegisterHandler(api)

	protected := api.Group("", httpx.RequireUser())
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
	} {
		handler.RegisterHandler(protected)
	}
	return engine
}
