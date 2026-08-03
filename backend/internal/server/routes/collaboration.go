package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

const collaborationWebSocketPath = "/api/v1/collaboration/ws"

// RegisterCollaborationRoutes registers capability discovery and the
// feature-gated, Panel-JWT-authenticated collaboration surface.
func RegisterCollaborationRoutes(
	v1 *gin.RouterGroup,
	cfg *config.Config,
	h *handler.CollaborationHandler,
	jwtAuth servermiddleware.JWTAuthMiddleware,
) {
	collaboration := v1.Group("/collaboration")
	collaboration.Use(gin.HandlerFunc(jwtAuth))

	collaboration.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "success",
			"data": gin.H{
				"enabled":                    cfg.Collaboration.Enabled,
				"protocol_version":           cfg.Collaboration.ProtocolVersion,
				"heartbeat_interval_seconds": cfg.Collaboration.HeartbeatIntervalSeconds,
				"websocket_path":             collaborationWebSocketPath,
			},
		})
	})

	if h == nil {
		return
	}
	devices := collaboration.Group("/devices")
	devices.Use(func(c *gin.Context) {
		if !cfg.Collaboration.Enabled {
			response.ErrorWithDetails(
				c,
				http.StatusServiceUnavailable,
				"Collaboration is disabled",
				"COLLABORATION_DISABLED",
				nil,
			)
			c.Abort()
			return
		}
		c.Next()
	})
	devices.POST("/register", h.RegisterDevice)
	devices.GET("", h.ListDevices)
	devices.PATCH("/:device_id", h.RenameDevice)
	devices.DELETE("/:device_id", h.RevokeDevice)
}
