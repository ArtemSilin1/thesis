package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4/pgxpool"
)

type Handler struct {
	db *pgxpool.Pool
}

func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{
		db: db,
	}
}

func (h *Handler) InitHandler(router *gin.Engine) {
	router.POST("/acc/create-acc", h.CreateAcc)
	router.POST("/acc/login", h.Login)
}

func (h *Handler) CreateAcc(c *gin.Context) {

}

func (h *Handler) Login(c *gin.Context) {

}
