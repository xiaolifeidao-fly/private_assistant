// Package reminders 提供 App 端的提醒、行程和 Web Push 订阅接口。
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
	group.GET("/trips/:key", h.trip)
	group.POST("/trips/:key/cancel", h.cancelTrip)
	group.GET("/push/config", h.pushConfig)
	group.POST("/push/subscriptions", h.subscribe)
	group.POST("/push/unsubscribe", h.unsubscribe)
	group.POST("/push/test", h.testPush)
}

func principal(c *gin.Context) httpx.Principal { return httpx.MustUser(c) }

func (h *Handler) list(c *gin.Context) {
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	result, err := h.reminders.Upcoming(c.Request.Context(), principal(c).FamilyKey, days)
	httpx.JSON(c, result, err)
}

func (h *Handler) create(c *gin.Context) {
	var body reminder.ManualInput
	if !httpx.Bind(c, &body) {
		return
	}
	result, err := h.reminders.CreateManual(c.Request.Context(), principal(c).FamilyKey, body)
	httpx.JSON(c, result, err)
}

func (h *Handler) dismiss(c *gin.Context) {
	httpx.JSON(c, gin.H{"ok": true}, h.reminders.Dismiss(c.Request.Context(), principal(c).FamilyKey, c.Param("key")))
}

func (h *Handler) trips(c *gin.Context) {
	result, err := h.reminders.ListTrips(c.Request.Context(), principal(c).FamilyKey, false)
	httpx.JSON(c, result, err)
}

func (h *Handler) createTrip(c *gin.Context) {
	var body reminder.TripInput
	if !httpx.Bind(c, &body) {
		return
	}
	result, err := h.reminders.CreateTrip(c.Request.Context(), principal(c).FamilyKey, body)
	if err == nil {
		_ = h.reminders.SyncDerived(c.Request.Context(), principal(c).FamilyKey, time.Now())
	}
	httpx.JSON(c, result, err)
}

func (h *Handler) trip(c *gin.Context) {
	result, err := h.reminders.GetTrip(c.Request.Context(), principal(c).FamilyKey, c.Param("key"), true)
	httpx.JSON(c, result, err)
}

func (h *Handler) cancelTrip(c *gin.Context) {
	httpx.JSON(c, gin.H{"ok": true}, h.reminders.CancelTrip(c.Request.Context(), principal(c).FamilyKey, c.Param("key")))
}

func (h *Handler) pushConfig(c *gin.Context) {
	httpx.JSON(c, gin.H{"enabled": h.reminders.PushEnabled(), "publicKey": h.reminders.PublicKey()}, nil)
}

func (h *Handler) subscribe(c *gin.Context) {
	var body reminder.PushSubscription
	if !httpx.Bind(c, &body) {
		return
	}
	user := principal(c)
	err := h.reminders.Subscribe(c.Request.Context(), user.FamilyKey, user.AccountKey, c.GetHeader("User-Agent"), body)
	httpx.JSON(c, gin.H{"ok": err == nil}, err)
}

func (h *Handler) unsubscribe(c *gin.Context) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	user := principal(c)
	err := h.reminders.Unsubscribe(c.Request.Context(), user.FamilyKey, user.AccountKey, body.Endpoint)
	httpx.JSON(c, gin.H{"ok": err == nil}, err)
}

func (h *Handler) testPush(c *gin.Context) {
	httpx.JSON(c, gin.H{"ok": true}, h.reminders.TestPush(c.Request.Context(), principal(c).FamilyKey))
}
