package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCollaborationHealthRequiresJWTAndReturnsCapabilities(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	authCalled := false

	RegisterCollaborationRoutes(
		router.Group("/api/v1"),
		&config.Config{Collaboration: config.CollaborationConfig{
			Enabled:                  true,
			ProtocolVersion:          1,
			HeartbeatIntervalSeconds: 20,
		}},
		nil,
		servermiddleware.JWTAuthMiddleware(func(c *gin.Context) {
			authCalled = true
			c.Next()
		}),
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/collaboration/health", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.True(t, authCalled)
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{
		"code": 0,
		"message": "success",
		"data": {
			"enabled": true,
			"protocol_version": 1,
			"heartbeat_interval_seconds": 20,
			"websocket_path": "/api/v1/collaboration/ws"
		}
	}`, response.Body.String())
}
