package handler

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	collaborationservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	collaborationCloseTryAgainLater    = 1013
	collaborationWebSocketWriteTimeout = 10 * time.Second
)

type CollaborationHandler struct {
	service                  *collaborationservice.Service
	heartbeatIntervalSeconds int
	eventProtocolVersion     int
	presenceTTL              time.Duration
	maxEventBytes            int64
	eventBus                 collaborationservice.EventBus
	connectionLeases         collaborationservice.ConnectionLeaseStore
	now                      func() time.Time
	upgrader                 websocket.Upgrader
}

func NewCollaborationHandler(
	service *collaborationservice.Service,
	cfg *config.Config,
	eventBus collaborationservice.EventBus,
	connectionLeases collaborationservice.ConnectionLeaseStore,
) *CollaborationHandler {
	presenceTTL := 45 * time.Second
	if cfg.Collaboration.PresenceTTLSeconds > 0 {
		presenceTTL = time.Duration(cfg.Collaboration.PresenceTTLSeconds) * time.Second
	}
	maxEventBytes := cfg.Collaboration.MaxEventBytes
	if maxEventBytes <= 0 {
		maxEventBytes = 1024 * 1024
	}
	return &CollaborationHandler{
		service:                  service,
		heartbeatIntervalSeconds: cfg.Collaboration.HeartbeatIntervalSeconds,
		eventProtocolVersion:     cfg.Collaboration.ProtocolVersion,
		presenceTTL:              presenceTTL,
		maxEventBytes:            maxEventBytes,
		eventBus:                 eventBus,
		connectionLeases:         connectionLeases,
		now:                      time.Now,
		upgrader: websocket.Upgrader{
			HandshakeTimeout: collaborationWebSocketWriteTimeout,
			CheckOrigin:      collaborationWebSocketOriginAllowed,
		},
	}
}

type collaborationRegisterDeviceRequest struct {
	InstallationIDHash string          `json:"installation_id_hash" binding:"required"`
	Name               string          `json:"name" binding:"required"`
	Platform           string          `json:"platform" binding:"required"`
	PlatformVersion    *string         `json:"platform_version"`
	CompanionVersion   string          `json:"companion_version" binding:"required"`
	CodexVersion       *string         `json:"codex_version"`
	ProtocolVersion    int             `json:"protocol_version" binding:"required"`
	Capabilities       map[string]bool `json:"capabilities"`
}

type collaborationRenameDeviceRequest struct {
	Name string `json:"name" binding:"required"`
}

type collaborationDeviceResponse struct {
	ID               uuid.UUID       `json:"id"`
	Name             string          `json:"name"`
	Platform         string          `json:"platform"`
	PlatformVersion  *string         `json:"platform_version,omitempty"`
	CompanionVersion string          `json:"companion_version"`
	CodexVersion     *string         `json:"codex_version,omitempty"`
	ProtocolVersion  int             `json:"protocol_version"`
	Status           string          `json:"status"`
	Capabilities     map[string]bool `json:"capabilities"`
	LastSeenAt       *time.Time      `json:"last_seen_at,omitempty"`
	RegisteredAt     time.Time       `json:"registered_at"`
}

func collaborationDeviceDTO(device collaborationservice.Device) collaborationDeviceResponse {
	return collaborationDeviceResponse{
		ID:               device.ID,
		Name:             device.Name,
		Platform:         device.Platform,
		PlatformVersion:  device.PlatformVersion,
		CompanionVersion: device.CompanionVersion,
		CodexVersion:     device.CodexVersion,
		ProtocolVersion:  device.ProtocolVersion,
		Status:           string(device.Status),
		Capabilities:     device.Capabilities,
		LastSeenAt:       device.LastSeenAt,
		RegisteredAt:     device.RegisteredAt,
	}
}

func (h *CollaborationHandler) RegisterDevice(c *gin.Context) {
	subject, ok := servermiddleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var request collaborationRegisterDeviceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid device registration")
		return
	}
	device, err := h.service.RegisterDevice(c.Request.Context(), subject.UserID, collaborationservice.RegisterDeviceInput{
		InstallationIDHash: request.InstallationIDHash,
		Name:               request.Name,
		Platform:           request.Platform,
		PlatformVersion:    request.PlatformVersion,
		CompanionVersion:   request.CompanionVersion,
		CodexVersion:       request.CodexVersion,
		ProtocolVersion:    request.ProtocolVersion,
		Capabilities:       request.Capabilities,
	})
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	response.Success(c, gin.H{
		"device_id":                  device.ID,
		"heartbeat_interval_seconds": h.heartbeatIntervalSeconds,
		"event_protocol_version":     h.eventProtocolVersion,
		"server_time":                h.now().UTC(),
	})
}

func (h *CollaborationHandler) ListDevices(c *gin.Context) {
	subject, ok := servermiddleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	devices, err := h.service.ListDevices(c.Request.Context(), subject.UserID)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	items := make([]collaborationDeviceResponse, 0, len(devices))
	for _, device := range devices {
		items = append(items, collaborationDeviceDTO(device))
	}
	response.Success(c, gin.H{"items": items})
}

func (h *CollaborationHandler) RenameDevice(c *gin.Context) {
	subject, deviceID, ok := collaborationDeviceRequestContext(c)
	if !ok {
		return
	}
	var request collaborationRenameDeviceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid device update")
		return
	}
	device, err := h.service.RenameDevice(c.Request.Context(), subject.UserID, deviceID, request.Name)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	response.Success(c, collaborationDeviceDTO(device))
}

func (h *CollaborationHandler) RevokeDevice(c *gin.Context) {
	subject, deviceID, ok := collaborationDeviceRequestContext(c)
	if !ok {
		return
	}
	_, err := h.service.RevokeDevice(c.Request.Context(), subject.UserID, deviceID)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func collaborationDeviceRequestContext(c *gin.Context) (servermiddleware.AuthSubject, uuid.UUID, bool) {
	subject, ok := servermiddleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return servermiddleware.AuthSubject{}, uuid.Nil, false
	}
	deviceID, err := uuid.Parse(c.Param("device_id"))
	if err != nil {
		response.BadRequest(c, "Invalid device ID")
		return servermiddleware.AuthSubject{}, uuid.Nil, false
	}
	return subject, deviceID, true
}

func writeCollaborationError(c *gin.Context, err error) {
	var insufficientBalance *collaborationservice.InsufficientBalanceError
	switch {
	case errors.Is(err, collaborationservice.ErrInvalidArgument):
		response.BadRequest(c, "Invalid collaboration request")
	case errors.Is(err, collaborationservice.ErrIdempotencyConflict):
		response.ErrorWithDetails(c, http.StatusConflict, "Idempotency key conflicts with an earlier request", "COLLAB_IDEMPOTENCY_CONFLICT", nil)
	case errors.Is(err, collaborationservice.ErrDeviceOffline):
		response.ErrorWithDetails(c, http.StatusConflict, "Collaboration device is offline", "COLLAB_DEVICE_OFFLINE", nil)
	case errors.Is(err, collaborationservice.ErrDeviceCapability):
		response.ErrorWithDetails(c, http.StatusUnprocessableEntity, "Collaboration capability is unavailable", "COLLAB_CODEX_INCOMPATIBLE", nil)
	case errors.Is(err, collaborationservice.ErrRelayUnavailable):
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "Collaboration relay unavailable", "COLLAB_RELAY_UNAVAILABLE", nil)
	case errors.Is(err, collaborationservice.ErrPayloadNotFound):
		response.ErrorWithDetails(c, http.StatusGone, "Collaboration snapshot has expired", "COLLAB_SNAPSHOT_EXPIRED", nil)
	case errors.Is(err, collaborationservice.ErrInvalidTransition):
		response.ErrorWithDetails(c, http.StatusConflict, "Collaboration state has changed", "COLLAB_STATE_CONFLICT", nil)
	case errors.Is(err, collaborationservice.ErrConnectionLimit):
		response.ErrorWithDetails(c, http.StatusTooManyRequests, "Too many collaboration connections", "COLLAB_CONNECTION_LIMIT", nil)
	case errors.Is(err, collaborationservice.ErrCommandRateLimit):
		response.ErrorWithDetails(c, http.StatusTooManyRequests, "Too many collaboration commands", "COLLAB_RATE_LIMITED", nil)
	case errors.As(err, &insufficientBalance):
		response.ErrorWithDetails(c, http.StatusConflict, "Insufficient balance", "COLLAB_INSUFFICIENT_BALANCE", map[string]string{
			"available_balance": insufficientBalance.Available.String(),
		})
	case errors.Is(err, collaborationservice.ErrProtocolMismatch):
		response.ErrorWithDetails(c, http.StatusConflict, "Unsupported collaboration protocol", "PROTOCOL_MISMATCH", nil)
	case errors.Is(err, collaborationservice.ErrDeviceRevoked):
		response.ErrorWithDetails(c, http.StatusConflict, "Device has been revoked", "DEVICE_REVOKED", nil)
	case errors.Is(err, collaborationservice.ErrNotFound):
		response.NotFound(c, "Collaboration resource not found")
	default:
		response.InternalError(c, "Collaboration request failed")
	}
}

func (h *CollaborationHandler) WebSocket(c *gin.Context) {
	subject, ok := servermiddleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	clientType := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Sub2API-Client-Type")))
	if clientType != "pc" && clientType != "mobile" {
		response.BadRequest(c, "Invalid collaboration client type")
		return
	}
	protocolVersion, err := strconv.Atoi(strings.TrimSpace(c.GetHeader("X-Sub2API-Protocol-Version")))
	if err != nil || protocolVersion != h.eventProtocolVersion {
		response.ErrorWithDetails(c, http.StatusUpgradeRequired, "Unsupported collaboration protocol", "PROTOCOL_MISMATCH", nil)
		return
	}
	if h.eventBus == nil || h.connectionLeases == nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "Collaboration relay unavailable", "COLLAB_RELAY_UNAVAILABLE", nil)
		return
	}

	requestContext, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	var (
		deviceID     uuid.UUID
		subscription collaborationservice.EventSubscription
	)
	lease := collaborationservice.ConnectionLease{
		UserID:       subject.UserID,
		ConnectionID: uuid.New(),
	}
	if clientType == "pc" {
		deviceID, err = uuid.Parse(strings.TrimSpace(c.GetHeader("X-Sub2API-Device-ID")))
		if err != nil {
			response.BadRequest(c, "Invalid collaboration device ID")
			return
		}
		if _, err := h.service.AuthenticateDevice(requestContext, subject.UserID, deviceID); err != nil {
			writeCollaborationError(c, err)
			return
		}
		lease.DeviceID = deviceID
	} else if strings.TrimSpace(c.GetHeader("X-Sub2API-Device-ID")) != "" {
		response.BadRequest(c, "Mobile collaboration connections cannot claim a device")
		return
	}
	acquired, err := h.connectionLeases.Acquire(requestContext, lease)
	if err != nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "Collaboration relay unavailable", "COLLAB_RELAY_UNAVAILABLE", nil)
		return
	}
	if !acquired {
		response.ErrorWithDetails(c, http.StatusTooManyRequests, "Too many collaboration connections", "COLLAB_CONNECTION_LIMIT", nil)
		return
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		// The lease also expires automatically; release is best effort.
		_ = h.connectionLeases.Release(cleanupCtx, lease)
		cleanupCancel()
	}()

	if clientType == "pc" {
		subscription, err = h.eventBus.SubscribeDevice(requestContext, deviceID)
	} else {
		subscription, err = h.eventBus.SubscribeUser(requestContext, subject.UserID)
	}
	if err != nil {
		response.ErrorWithDetails(c, http.StatusServiceUnavailable, "Collaboration relay unavailable", "COLLAB_RELAY_UNAVAILABLE", nil)
		return
	}
	defer func() { _ = subscription.Close() }()

	connection, err := h.upgrader.Upgrade(c.Writer, c.Request, servermiddleware.ServerTimingResponseHeader(c))
	if err != nil {
		return
	}
	defer func() { _ = connection.Close() }()
	connection.SetReadLimit(h.maxEventBytes)
	_ = connection.SetReadDeadline(time.Now().Add(2 * h.presenceTTL))
	connection.SetPongHandler(func(string) error {
		return connection.SetReadDeadline(time.Now().Add(2 * h.presenceTTL))
	})

	writerDone := make(chan struct{})
	go h.writeCollaborationEvents(requestContext, connection, subscription.Events(), writerDone)
	h.readCollaborationEvents(requestContext, connection, clientType, lease)
	cancel()
	<-writerDone

	if clientType == "pc" {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		h.service.RecordDisconnect(cleanupCtx, subject.UserID, deviceID)
		_, _ = h.eventBus.PublishUser(cleanupCtx, subject.UserID, "device.presence_changed", nil, map[string]any{
			"device_id": deviceID.String(),
			"status":    "offline",
		})
		cleanupCancel()
	}
}

func (h *CollaborationHandler) readCollaborationEvents(
	ctx context.Context,
	connection *websocket.Conn,
	clientType string,
	lease collaborationservice.ConnectionLease,
) {
	for {
		var event collaborationservice.EventEnvelope
		if err := connection.ReadJSON(&event); err != nil {
			return
		}
		if ctx.Err() != nil || event.Version != 1 || !collaborationservice.ValidEventType(event.Type) || len(event.EventID) < 8 || len(event.Payload) > 64 {
			return
		}
		if event.Type == "heartbeat" {
			renewed, err := h.connectionLeases.Renew(ctx, lease)
			if err != nil || !renewed {
				return
			}
		}
		if clientType == "mobile" {
			if event.Type != "heartbeat" {
				return
			}
			_, _ = h.eventBus.PublishUser(ctx, lease.UserID, "heartbeat.ack", event.RequestID, map[string]any{
				"server_time": h.now().UTC(),
			})
			continue
		}
		if event.Type == "device.hello" {
			continue
		}
		if event.Type != "heartbeat" {
			if err := h.service.HandleDeviceEvent(ctx, lease.UserID, lease.DeviceID, event); err != nil {
				return
			}
			continue
		}
		appServerStatus, ok := event.Payload["app_server_status"].(string)
		if !ok {
			return
		}
		activeThreadCount, ok := collaborationEventInteger(event.Payload["active_thread_count"])
		if !ok {
			return
		}
		presence, err := h.service.RecordHeartbeat(ctx, lease.UserID, lease.DeviceID, appServerStatus, activeThreadCount)
		if err != nil {
			return
		}
		_ = connection.SetReadDeadline(time.Now().Add(2 * h.presenceTTL))
		_, _ = h.eventBus.PublishDevice(ctx, lease.UserID, lease.DeviceID, "heartbeat.ack", event.RequestID, map[string]any{
			"server_time": h.now().UTC(),
		})
		_, _ = h.eventBus.PublishUser(ctx, lease.UserID, "device.presence_changed", event.RequestID, map[string]any{
			"device_id":    lease.DeviceID.String(),
			"status":       presence.Status,
			"last_seen_at": presence.LastSeenAt,
		})
	}
}

func (h *CollaborationHandler) writeCollaborationEvents(
	ctx context.Context,
	connection *websocket.Conn,
	events <-chan collaborationservice.EventEnvelope,
	done chan<- struct{},
) {
	defer close(done)
	defer func() { _ = connection.Close() }()
	for {
		select {
		case <-ctx.Done():
			_ = connection.SetWriteDeadline(time.Now().Add(collaborationWebSocketWriteTimeout))
			_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "closed"), time.Now().Add(collaborationWebSocketWriteTimeout))
			return
		case event, ok := <-events:
			if !ok {
				_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(collaborationCloseTryAgainLater, "relay overflow"), time.Now().Add(collaborationWebSocketWriteTimeout))
				return
			}
			_ = connection.SetWriteDeadline(time.Now().Add(collaborationWebSocketWriteTimeout))
			if err := connection.WriteJSON(event); err != nil {
				return
			}
		}
	}
}

func collaborationWebSocketOriginAllowed(request *http.Request) bool {
	origin := strings.TrimSpace(request.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && strings.EqualFold(parsed.Host, request.Host) && (parsed.Scheme == "https" || parsed.Scheme == "http")
}

func collaborationEventInteger(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		converted := int(typed)
		return converted, typed == float64(converted)
	case int:
		return typed, true
	case int64:
		return int(typed), true
	default:
		return 0, false
	}
}
