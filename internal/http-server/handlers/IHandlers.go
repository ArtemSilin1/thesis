package handlers

import "github.com/gin-gonic/gin"

type Handler interface {
	InitHandler(router *gin.Engine)
}
