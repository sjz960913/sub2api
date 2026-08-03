package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	collaborationservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CollaborationHandler struct {
	service                  *collaborationservice.Service
	heartbeatIntervalSeconds int
	eventProtocolVersion     int
	now                      func() time.Time
}

func NewCollaborationHandler(
	service *collaborationservice.Service,
	cfg *config.Config,
) *CollaborationHandler {
	return &CollaborationHandler{
		service:                  service,
		heartbeatIntervalSeconds: cfg.Collaboration.HeartbeatIntervalSeconds,
		eventProtocolVersion:     cfg.Collaboration.ProtocolVersion,
		now:                      time.Now,
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
	switch {
	case errors.Is(err, collaborationservice.ErrInvalidArgument):
		response.BadRequest(c, "Invalid collaboration request")
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
