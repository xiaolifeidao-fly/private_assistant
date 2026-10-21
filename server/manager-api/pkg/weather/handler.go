// Package weather 是管理端的宝宝穿衣：看今天的天气和提醒、试一下 / 现在推送、改设置和送接安排（service/weather）。
package weather

import (
	"encoding/json"

	"common/middleware/httpx"
	"service/weather"

	"github.com/gin-gonic/gin"
)

type Handler struct{ weather *weather.Service }

func New(service *weather.Service) *Handler { return &Handler{weather: service} }

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/weather", h.today)
	group.GET("/weather/status", h.status)
	group.POST("/weather/refresh", h.refresh)
	group.GET("/weather/briefs", h.briefs)
	group.POST("/weather/preview", h.preview)
	group.POST("/weather/send", h.send)
	group.GET("/weather/days", h.days)
	group.PUT("/weather/day", h.saveDay)
	group.DELETE("/weather/day", h.resetDay)
	group.GET("/weather/config", h.config)
	group.PUT("/weather/config", h.saveConfig)
	group.POST("/weather/config/reset", h.resetConfig)
	group.GET("/weather/locations", h.searchLocations)
}

func (h *Handler) today(c *gin.Context) {
	result, err := h.weather.Today(c.Request.Context(), httpx.MustUser(c).FamilyKey)
	httpx.JSON(c, result, err)
}

func (h *Handler) status(c *gin.Context) {
	httpx.JSON(c, h.weather.Status(), nil)
}

// refresh 不用缓存重新取一次天气，返回今天的全部。
func (h *Handler) refresh(c *gin.Context) {
	ctx := c.Request.Context()
	if _, err := h.weather.SnapshotAt(ctx, h.weather.Config(ctx).Location, true); err != nil {
		httpx.JSON(c, nil, err)
		return
	}
	result, err := h.weather.Today(ctx, httpx.MustUser(c).FamilyKey)
	httpx.JSON(c, result, err)
}

// briefs ?from=&to= 过去的提醒。
func (h *Handler) briefs(c *gin.Context) {
	result, err := h.weather.Briefs(c.Request.Context(), httpx.MustUser(c).FamilyKey, c.Query("from"), c.Query("to"))
	httpx.JSON(c, gin.H{"items": result}, err)
}

type slotBody struct {
	Slot string `json:"slot"`
	Date string `json:"date"`
}

// preview 生成但不存不推。
func (h *Handler) preview(c *gin.Context) {
	var body slotBody
	if !httpx.Bind(c, &body) {
		return
	}
	result, err := h.weather.Preview(c.Request.Context(), httpx.MustUser(c).FamilyKey, body.Slot, body.Date)
	httpx.JSON(c, result, err)
}

// send 生成并推送（覆盖这天这个时段已有的）。
func (h *Handler) send(c *gin.Context) {
	var body slotBody
	if !httpx.Bind(c, &body) {
		return
	}
	user := httpx.MustUser(c)
	result, err := h.weather.Send(c.Request.Context(), user.FamilyKey, body.Slot, body.Date, user.AccountKey)
	httpx.JSON(c, result, err)
}

// days ?from=&days=7 这几天的送接安排。
func (h *Handler) days(c *gin.Context) {
	result, err := h.weather.ListDays(c.Request.Context(), httpx.MustUser(c).FamilyKey, c.Query("from"), httpx.QueryInt(c, "days", 7))
	httpx.JSON(c, gin.H{"items": result}, err)
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

func (h *Handler) resetDay(c *gin.Context) {
	result, err := h.weather.ResetDay(c.Request.Context(), httpx.MustUser(c).FamilyKey, c.Query("date"))
	httpx.JSON(c, result, err)
}

func (h *Handler) config(c *gin.Context) {
	httpx.JSON(c, gin.H{"config": h.weather.Config(c.Request.Context()), "defaults": weather.DefaultConfig()}, nil)
}

func (h *Handler) saveConfig(c *gin.Context) {
	var raw json.RawMessage
	if !httpx.Bind(c, &raw) {
		return
	}
	var body weather.Config
	if err := json.Unmarshal(raw, &body); err != nil {
		httpx.Fail(c, "格式不对")
		return
	}
	result, err := h.weather.SaveConfig(c.Request.Context(), body)
	httpx.JSON(c, gin.H{"config": result}, err)
}

func (h *Handler) resetConfig(c *gin.Context) {
	result, err := h.weather.ResetConfig(c.Request.Context())
	httpx.JSON(c, gin.H{"config": result}, err)
}

func (h *Handler) searchLocations(c *gin.Context) {
	result, err := h.weather.SearchLocations(c.Request.Context(), c.Query("q"))
	httpx.JSON(c, gin.H{"items": result}, err)
}
