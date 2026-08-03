package routes

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

const collaborationWebSocketPath = "/api/v1/collaboration/ws"

// RegisterCollaborationRoutes registers the versioned mobile/PC collaboration
// surface. M0 intentionally exposes only capability discovery; device state,
// command billing and realtime relay are added behind the same feature flag in
// later milestones.
func RegisterCollaborationRoutes(
	v1 *gin.RouterGroup,
	cfg *config.Config,
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
}
