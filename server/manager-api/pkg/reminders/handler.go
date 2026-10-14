// Package reminders 提供管理端的提醒、行程和推送日志查询。
package reminders

import (
	"strconv"
	"time"

	"common/middleware/httpx"
	"service/reminder"

	"github.com/gin-gonic/gin"
)

type Handler struct{ reminders *reminder.Service }

func New(service *reminder.Service) *Handler { return &Handler{reminders: service} }

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/reminders", h.list)
	group.POST("/reminders", h.create)
	group.POST("/reminders/:key/dismiss", h.dismiss)
	group.GET("/trips", h.trips)
	group.POST("/trips", h.createTrip)
	group.GET("/push-deliveries", h.deliveries)
}

func familyKey(c *gin.Context) string { return httpx.MustUser(c).FamilyKey }

func (h *Handler) list(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "90"))
	now := time.Now()
	result, err := h.reminders.List(c.Request.Context(), familyKey(c), now.AddDate(0, 0, -30), now.AddDate(0, 0, days), true)
	httpx.JSON(c, result, err)
}

func (h *Handler) create(c *gin.Context) {
	var body reminder.ManualInput
	if !httpx.Bind(c, &body) {
		return
	}
	result, err := h.reminders.CreateManual(c.Request.Context(), familyKey(c), body)
	httpx.JSON(c, result, err)
}

func (h *Handler) dismiss(c *gin.Context) {
	httpx.JSON(c, gin.H{"ok": true}, h.reminders.Dismiss(c.Request.Context(), familyKey(c), c.Param("key")))
}

func (h *Handler) trips(c *gin.Context) {
	result, err := h.reminders.ListTrips(c.Request.Context(), familyKey(c), true)
	httpx.JSON(c, result, err)
}

func (h *Handler) createTrip(c *gin.Context) {
	var body reminder.TripInput
	if !httpx.Bind(c, &body) {
		return
	}
	result, err := h.reminders.CreateTrip(c.Request.Context(), familyKey(c), body)
	httpx.JSON(c, result, err)
}

func (h *Handler) deliveries(c *gin.Context) {
	result, err := h.reminders.ListDeliveries(c.Request.Context(), familyKey(c))
	httpx.JSON(c, result, err)
}
