package httpapi

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

func (s *Server) SetReady(ready bool) { s.ready.Store(ready) }
func (s *Server) AttachRuntime(callbacks http.Handler) {
	s.router.Any("/callbacks/algorithms/*path", s.timeout, gin.WrapH(callbacks))
	s.router.GET("/algorithm-assets/*path", s.timeout, gin.WrapH(callbacks))
	s.router.HEAD("/algorithm-assets/*path", s.timeout, gin.WrapH(callbacks))
	s.router.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"live": true}) })
	s.router.GET("/readyz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if !s.ready.Load() || s.db.PingContext(ctx) != nil {
			s.failure(c, 503, "NOT_READY")
			return
		}
		c.JSON(200, gin.H{"ready": true})
	})
}
