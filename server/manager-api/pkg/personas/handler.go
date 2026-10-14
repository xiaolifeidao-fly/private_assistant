// Package personas 是人设卡：查看当前版本和历史、立即重新生成、手工修订、回滚、对话补充。
package personas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"common/middleware/httpx"
	"service/persona"

	"github.com/gin-gonic/gin"
)

type Handler struct{ persona *persona.Service }

func New(service *persona.Service) *Handler { return &Handler{persona: service} }

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/persona-cards", h.history)
	group.POST("/persona-cards", h.saveManual)
	group.POST("/persona-cards/generate", h.generate)
	group.POST("/persona-cards/:key/activate", h.activate)
	group.POST("/persona-cards/interview", h.interview)
	group.POST("/persona-cards/interview/draft", h.draft)
	group.POST("/persona-cards/interview/save", h.saveInterview)
}

func familyKey(c *gin.Context) string { return httpx.MustUser(c).FamilyKey }

func (h *Handler) history(c *gin.Context) {
	result, err := h.persona.History(c.Request.Context(), familyKey(c), c.Query("person"))
	httpx.JSON(c, result, err)
}

func (h *Handler) saveManual(c *gin.Context) {
	var body struct {
		Person  string `json:"person"`
		Content string `json:"content"`
		Changes string `json:"changes"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	result, err := h.persona.SaveManual(c.Request.Context(), familyKey(c), body.Person, body.Content, body.Changes)
	httpx.JSON(c, result, err)
}

// generate 立即重新生成（强制），同步等待模型返回，一般几十秒。
func (h *Handler) generate(c *gin.Context) {
	var body struct {
		Person string `json:"person"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Minute)
	defer cancel()
	card, changed, err := h.persona.Generate(ctx, familyKey(c), body.Person, true)
	httpx.JSON(c, gin.H{"card": card, "changed": changed}, err)
}

func (h *Handler) activate(c *gin.Context) {
	result, err := h.persona.Activate(c.Request.Context(), familyKey(c), c.Param("key"))
	httpx.JSON(c, result, err)
}

type interviewBody struct {
	Person string         `json:"person"`
	Turns  []persona.Turn `json:"turns"`
}

// interviewEvent 是对话补充的 SSE 事件：delta（回复增量）、done（完整回复）、error。
type interviewEvent struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

// interview 以 SSE 推送模型的下一轮提问，每个事件一行 data: {json}。对话不落库，断开就丢，所以跟着请求取消。
func (h *Handler) interview(c *gin.Context) {
	var body interviewBody
	if !httpx.Bind(c, &body) {
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 关掉 nginx 缓冲，否则流式变成一次性返回
	c.Status(http.StatusOK)

	var mu sync.Mutex
	write := func(event interviewEvent) {
		mu.Lock()
		defer mu.Unlock()
		data, _ := json.Marshal(event)
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err == nil {
			c.Writer.Flush()
		}
	}

	// 定时发注释行保活，防止代理在模型思考期间断开空闲连接。
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				mu.Lock()
				if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err == nil {
					c.Writer.Flush()
				}
				mu.Unlock()
			}
		}
	}()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	reply, err := h.persona.Interview(ctx, familyKey(c), body.Person, body.Turns, func(text string) {
		write(interviewEvent{Type: "delta", Text: text})
	})
	if err != nil {
		write(interviewEvent{Type: "error", Error: err.Error()})
		return
	}
	write(interviewEvent{Type: "done", Text: reply})
}

// draft 把对话整理成人设卡草稿，不保存。同步等待模型返回，一般几十秒。
func (h *Handler) draft(c *gin.Context) {
	var body interviewBody
	if !httpx.Bind(c, &body) {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Minute)
	defer cancel()
	result, err := h.persona.DraftFromInterview(ctx, familyKey(c), body.Person, body.Turns)
	httpx.JSON(c, result, err)
}

func (h *Handler) saveInterview(c *gin.Context) {
	var body struct {
		Person  string `json:"person"`
		Content string `json:"content"`
		Changes string `json:"changes"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	result, err := h.persona.SaveInterview(c.Request.Context(), familyKey(c), body.Person, body.Content, body.Changes)
	httpx.JSON(c, result, err)
}
