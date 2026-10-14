// Package chats 是 App 的对话：机器人列表、会话、消息（SSE 流式）和反馈。
package chats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"common/middleware/httpx"
	"service/bot"
	"service/chat"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	chat *chat.Service
	bots *bot.Service
}

func New(chatService *chat.Service, botService *bot.Service) *Handler {
	return &Handler{chat: chatService, bots: botService}
}

func (h *Handler) RegisterHandler(group *gin.RouterGroup) {
	group.GET("/bots", h.listBots)
	group.GET("/conversations", h.list)
	group.POST("/conversations", h.create)
	group.GET("/conversations/:key", h.get)
	group.PUT("/conversations/:key", h.rename)
	group.DELETE("/conversations/:key", h.archive)
	group.GET("/conversations/:key/messages", h.messages)
	group.POST("/conversations/:key/messages", h.send)
	group.POST("/messages/:key/feedback", h.feedback)
}

func owner(c *gin.Context) chat.Owner {
	principal := httpx.MustUser(c)
	return chat.Owner{FamilyKey: principal.FamilyKey, AccountKey: principal.AccountKey}
}

// listBots 只返回 App 需要的字段，不下发系统提示词。
func (h *Handler) listBots(c *gin.Context) {
	items, err := h.bots.List(c.Request.Context(), owner(c).FamilyKey, true)
	if err != nil {
		httpx.JSON(c, nil, err)
		return
	}
	result := make([]gin.H, 0, len(items))
	for _, item := range items {
		result = append(result, gin.H{
			"botKey": item.BotKey, "name": item.Name, "kind": item.Kind, "personKey": item.PersonKey,
			"avatar": item.Avatar, "description": item.Description, "greeting": item.Greeting,
		})
	}
	httpx.JSON(c, result, nil)
}

func (h *Handler) list(c *gin.Context) {
	items, total, err := h.chat.ListConversations(c.Request.Context(), owner(c), c.Query("botKey"),
		httpx.QueryInt(c, "page", 1), httpx.QueryInt(c, "size", 30))
	httpx.JSON(c, gin.H{"items": items, "total": total}, err)
}

func (h *Handler) create(c *gin.Context) {
	var body struct {
		BotKey string `json:"botKey"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	result, err := h.chat.CreateConversation(c.Request.Context(), owner(c), body.BotKey)
	httpx.JSON(c, result, err)
}

func (h *Handler) get(c *gin.Context) {
	result, err := h.chat.GetConversation(c.Request.Context(), owner(c), c.Param("key"))
	httpx.JSON(c, result, err)
}

func (h *Handler) rename(c *gin.Context) {
	var body struct {
		Title string `json:"title"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	httpx.JSON(c, nil, h.chat.RenameConversation(c.Request.Context(), owner(c), c.Param("key"), body.Title))
}

func (h *Handler) archive(c *gin.Context) {
	httpx.JSON(c, nil, h.chat.ArchiveConversation(c.Request.Context(), owner(c), c.Param("key")))
}

func (h *Handler) messages(c *gin.Context) {
	result, err := h.chat.Messages(c.Request.Context(), owner(c), c.Param("key"))
	httpx.JSON(c, result, err)
}

func (h *Handler) feedback(c *gin.Context) {
	var body struct {
		Feedback string `json:"feedback"`
		Note     string `json:"note"`
	}
	if !httpx.Bind(c, &body) {
		return
	}
	httpx.JSON(c, nil, h.chat.Feedback(c.Request.Context(), owner(c), c.Param("key"), body.Feedback, body.Note))
}

// send 以 SSE 推送回答过程：每个事件一行 data: {json}。
//
// 回答跑在与请求解绑的 context 上：手机切到后台、网络断开时回答照样写完落库，
// 回到页面重新拉消息就能看到。
func (h *Handler) send(c *gin.Context) {
	var body chat.SendInput
	if !httpx.Bind(c, &body) {
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 关掉 nginx 缓冲，否则流式变成一次性返回
	c.Status(http.StatusOK)

	var mu sync.Mutex
	disconnected := false
	write := func(event chat.Event) {
		mu.Lock()
		defer mu.Unlock()
		if disconnected {
			return
		}
		data, _ := json.Marshal(event)
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
			disconnected = true
			return
		}
		c.Writer.Flush()
	}

	// 定时发注释行保活，防止代理在模型思考期间断开空闲连接。
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				mu.Lock()
				if !disconnected {
					if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
						disconnected = true
					} else {
						c.Writer.Flush()
					}
				}
				mu.Unlock()
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 15*time.Minute)
	defer cancel()
	err := h.chat.Send(ctx, owner(c), c.Param("key"), body, write)
	close(done)
	if err != nil {
		write(chat.Event{Type: "error", Error: err.Error()})
	}
}
