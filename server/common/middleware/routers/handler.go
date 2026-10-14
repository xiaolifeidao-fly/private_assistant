package routers

import "github.com/gin-gonic/gin"

// Handler 是每个子域 handler 的注册契约：*-api 的 router 只认这个接口。
type Handler interface {
	RegisterHandler(*gin.RouterGroup)
}
