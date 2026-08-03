package handler

import (
	"encoding/json"
	"strings"

	collabdomain "github.com/Wei-Shaw/sub2api/internal/domain/collaboration"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	collaborationservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type collaborationSessionSyncRequest struct {
	SearchTerm *string `json:"search_term"`
	CWD        *string `json:"cwd"`
	Archived   bool    `json:"archived"`
	Cursor     *string `json:"cursor"`
	Limit      int     `json:"limit"`
}

type collaborationThreadSyncRequest struct {
	AfterItemID       *string `json:"after_item_id"`
	Cursor            *string `json:"cursor"`
	Limit             int     `json:"limit"`
	IncludeToolOutput string  `json:"include_tool_output"`
}

type collaborationCommandRequest struct {
	DeviceID      uuid.UUID                   `json:"device_id" binding:"required"`
	ThreadID      string                      `json:"thread_id" binding:"required"`
	Input         []collaborationCommandInput `json:"input" binding:"required"`
	ClientContext *collaborationClientContext `json:"client_context"`
}

type collaborationCommandInput struct {
	Type string `json:"type" binding:"required"`
	Text string `json:"text" binding:"required"`
}

type collaborationClientContext struct {
	Locale string `json:"locale"`
	Source string `json:"source"`
}

func (h *CollaborationHandler) CreateSessionSync(c *gin.Context) {
	subject, deviceID, ok := collaborationDeviceRequestContext(c)
	if !ok {
		return
	}
	idempotencyKey, ok := collaborationIdempotencyKey(c)
	if !ok {
		return
	}
	var request collaborationSessionSyncRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid session sync request")
		return
	}
	if request.Limit == 0 {
		request.Limit = 50
	}
	if request.Limit < 1 || request.Limit > 100 || optionalStringTooLong(request.SearchTerm, 200) ||
		optionalStringTooLong(request.CWD, 1024) || optionalStringTooLong(request.Cursor, 1024) {
		response.BadRequest(c, "Invalid session sync request")
		return
	}
	result, err := h.service.RequestSync(c.Request.Context(), subject.UserID, collaborationservice.RequestSyncInput{
		DeviceID:       deviceID,
		IdempotencyKey: idempotencyKey,
		Kind:           collabdomain.SyncKindSessionList,
		Cursor:         request.Cursor,
		Payload: map[string]any{
			"search_term": request.SearchTerm,
			"cwd":         request.CWD,
			"archived":    request.Archived,
			"cursor":      request.Cursor,
			"limit":       request.Limit,
		},
	})
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	response.Accepted(c, collaborationSyncAcceptedDTO(result.Sync))
}

func (h *CollaborationHandler) CreateThreadSync(c *gin.Context) {
	subject, deviceID, ok := collaborationDeviceRequestContext(c)
	if !ok {
		return
	}
	idempotencyKey, ok := collaborationIdempotencyKey(c)
	if !ok {
		return
	}
	threadID := strings.TrimSpace(c.Param("thread_id"))
	var request collaborationThreadSyncRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "Invalid thread sync request")
		return
	}
	if request.Limit == 0 {
		request.Limit = 100
	}
	if request.IncludeToolOutput == "" {
		request.IncludeToolOutput = "summary"
	}
	if threadID == "" || len(threadID) > 512 || request.Limit < 1 || request.Limit > 200 ||
		optionalStringTooLong(request.AfterItemID, 512) || optionalStringTooLong(request.Cursor, 1024) ||
		(request.IncludeToolOutput != "none" && request.IncludeToolOutput != "summary") {
		response.BadRequest(c, "Invalid thread sync request")
		return
	}
	result, err := h.service.RequestSync(c.Request.Context(), subject.UserID, collaborationservice.RequestSyncInput{
		DeviceID:       deviceID,
		IdempotencyKey: idempotencyKey,
		Kind:           collabdomain.SyncKindThreadSnapshot,
		ThreadID:       &threadID,
		Cursor:         request.Cursor,
		Payload: map[string]any{
			"after_item_id":       request.AfterItemID,
			"cursor":              request.Cursor,
			"limit":               request.Limit,
			"include_tool_output": request.IncludeToolOutput,
		},
	})
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	response.Accepted(c, collaborationSyncAcceptedDTO(result.Sync))
}

func collaborationSyncAcceptedDTO(syncRequest collaborationservice.SyncRequest) gin.H {
	return gin.H{
		"sync_id":    syncRequest.ID,
		"status":     syncRequest.Status,
		"expires_at": syncRequest.ExpiresAt,
	}
}

func (h *CollaborationHandler) GetSessionSync(c *gin.Context) {
	h.getSync(c, collabdomain.SyncKindSessionList)
}

func (h *CollaborationHandler) GetThreadSync(c *gin.Context) {
	h.getSync(c, collabdomain.SyncKindThreadSnapshot)
}

func (h *CollaborationHandler) getSync(c *gin.Context, expectedKind collabdomain.SyncKind) {
	subject, ok := servermiddleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	syncID, err := uuid.Parse(c.Param("sync_id"))
	if err != nil {
		response.BadRequest(c, "Invalid sync ID")
		return
	}
	syncRequest, payload, err := h.service.GetSyncResult(c.Request.Context(), subject.UserID, syncID)
	if err != nil && syncRequest.ID == uuid.Nil {
		writeCollaborationError(c, err)
		return
	}
	if syncRequest.Kind != expectedKind {
		response.NotFound(c, "Collaboration resource not found")
		return
	}
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	data := gin.H{
		"sync_id":          syncRequest.ID,
		"status":           syncRequest.Status,
		"device_id":        syncRequest.DeviceID,
		"snapshot_version": syncRequest.SnapshotVersion,
		"items":            []any{},
		"error":            collaborationErrorSummary(syncRequest.ErrorCode),
	}
	if len(payload) > 0 {
		var snapshot map[string]any
		if err := json.Unmarshal(payload, &snapshot); err != nil {
			response.InternalError(c, "Collaboration sync payload is invalid")
			return
		}
		for key, value := range snapshot {
			if key != "sync_id" && key != "status" && key != "device_id" && key != "snapshot_version" && key != "error" {
				data[key] = value
			}
		}
	}
	response.Success(c, data)
}

func (h *CollaborationHandler) CreateCommand(c *gin.Context) {
	subject, ok := servermiddleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	idempotencyKey, ok := collaborationIdempotencyKey(c)
	if !ok {
		return
	}
	var request collaborationCommandRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.Input) != 1 ||
		request.Input[0].Type != "text" || strings.TrimSpace(request.Input[0].Text) == "" {
		response.BadRequest(c, "Invalid collaboration command")
		return
	}
	if request.ClientContext != nil && request.ClientContext.Source != "" && request.ClientContext.Source != "android" {
		response.BadRequest(c, "Invalid collaboration client context")
		return
	}
	result, err := h.service.DispatchCommand(c.Request.Context(), subject.UserID, collaborationservice.SubmitCommandInput{
		DeviceID:       request.DeviceID,
		ThreadID:       request.ThreadID,
		IdempotencyKey: idempotencyKey,
		Prompt:         request.Input[0].Text,
	})
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	response.Accepted(c, collaborationCommandDTO(result.Command))
}

func (h *CollaborationHandler) GetCommand(c *gin.Context) {
	subject, ok := servermiddleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	commandID, err := uuid.Parse(c.Param("command_id"))
	if err != nil {
		response.BadRequest(c, "Invalid command ID")
		return
	}
	command, err := h.service.GetCommand(c.Request.Context(), subject.UserID, commandID)
	if err != nil {
		writeCollaborationError(c, err)
		return
	}
	response.Success(c, collaborationCommandDTO(command))
}

func collaborationCommandDTO(command collaborationservice.Command) gin.H {
	return gin.H{
		"command_id": command.ID,
		"status":     command.Status,
		"turn_id":    command.TurnID,
		"error":      collaborationErrorSummary(command.ErrorCode),
		"created_at": command.CreatedAt,
		"updated_at": command.UpdatedAt,
	}
}

func collaborationErrorSummary(errorCode *string) any {
	if errorCode == nil {
		return nil
	}
	return gin.H{
		"reason":  *errorCode,
		"message": collaborationErrorMessage(*errorCode),
	}
}

func collaborationErrorMessage(errorCode string) string {
	switch errorCode {
	case "expired":
		return "Collaboration request expired"
	case "relay_unavailable":
		return "Collaboration relay unavailable"
	case "payload_unavailable":
		return "Collaboration payload unavailable"
	case "thread_read_only":
		return "Codex session is read-only"
	case "codex_unavailable":
		return "Codex is unavailable on the paired computer"
	default:
		return "Collaboration request failed"
	}
}

func collaborationIdempotencyKey(c *gin.Context) (uuid.UUID, bool) {
	idempotencyKey, err := uuid.Parse(strings.TrimSpace(c.GetHeader("Idempotency-Key")))
	if err != nil {
		response.BadRequest(c, "Invalid idempotency key")
		return uuid.Nil, false
	}
	return idempotencyKey, true
}

func optionalStringTooLong(value *string, limit int) bool {
	return value != nil && len(*value) > limit
}
