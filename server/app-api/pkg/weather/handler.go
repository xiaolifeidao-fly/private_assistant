// Package weather 是宝宝穿衣：天气、今天穿什么、几点前到家，和某一天的送接安排（service/weather）。
// 安排谁都能改（奶奶最清楚今天怎么安排）；查哪里的天气只有我能切，奶奶只看。
package weather

import (
	"common/middleware/httpx"
	"service/identity"
	"service/weather"

	"github.com/gin-gonic/gin"
)

type Handler struct{ weather *weather.Service }

func New(service *weather.Service) *Handler { return &Handler{weather: service} }

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/weather", h.today)
	group.PUT("/weather/day", h.saveDay)
	group.DELETE("/weather/day", h.resetDay)
	group.GET("/weather/locations", h.searchLocations)
	group.PUT("/weather/location", h.setLocation)
}

func (h *Handler) today(c *gin.Context) {
	user := httpx.MustUser(c)
	result, err := h.weather.Today(c.Request.Context(), user.FamilyKey)
	httpx.JSON(c, result, err)
}

func (h *Handler) saveDay(c *gin.Context) {
	var body weather.DayInput
	if !httpx.Bind(c, &body) {
		return
	}
	user := httpx.MustUser(c)
	result, err := h.weather.SaveDay(c.Request.Context(), user.FamilyKey, body, user.AccountKey)
	httpx.JSON(c, result, err)
}

// resetDay ?date=YYYY-MM-DD 这天恢复平常的安排。
func (h *Handler) resetDay(c *gin.Context) {
	user := httpx.MustUser(c)
	result, err := h.weather.ResetDay(c.Request.Context(), user.FamilyKey, c.Query("date"))
	httpx.JSON(c, result, err)
}

func (h *Handler) searchLocations(c *gin.Context) {
	result, err := h.weather.SearchLocations(c.Request.Context(), c.Query("q"))
	httpx.JSON(c, gin.H{"items": result}, err)
}

func (h *Handler) setLocation(c *gin.Context) {
	user := httpx.MustUser(c)
	if user.Role == identity.RoleElder || user.Role == identity.RoleSpouse {
		httpx.Fail(c, "换地方要在爸爸的手机上操作")
		return
	}
	var body weather.Location
	if !httpx.Bind(c, &body) {
		return
	}
	result, err := h.weather.SetLocation(c.Request.Context(), body)
	httpx.JSON(c, gin.H{"location": result.Location}, err)
}
