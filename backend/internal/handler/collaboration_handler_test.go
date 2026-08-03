package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	collaborationdomain "github.com/Wei-Shaw/sub2api/internal/domain/collaboration"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	collaborationservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type collaborationHandlerRepositoryStub struct {
	listUserID int64
	devices    []collaborationservice.Device
}

func (r *collaborationHandlerRepositoryStub) RegisterDevice(context.Context, int64, collaborationservice.RegisterDeviceInput) (collaborationservice.Device, error) {
	return collaborationservice.Device{}, nil
}

func (r *collaborationHandlerRepositoryStub) ListDevices(_ context.Context, userID int64) ([]collaborationservice.Device, error) {
	r.listUserID = userID
	return r.devices, nil
}

func (r *collaborationHandlerRepositoryStub) RenameDevice(context.Context, int64, uuid.UUID, string) (collaborationservice.Device, error) {
	return collaborationservice.Device{}, nil
}

func (r *collaborationHandlerRepositoryStub) RevokeDevice(context.Context, int64, uuid.UUID) (collaborationservice.Device, error) {
	return collaborationservice.Device{}, nil
}

func (r *collaborationHandlerRepositoryStub) GetDevice(context.Context, int64, uuid.UUID) (collaborationservice.Device, error) {
	return collaborationservice.Device{}, nil
}

func (r *collaborationHandlerRepositoryStub) UpdateDevicePresence(context.Context, int64, uuid.UUID, collaborationdomain.DeviceStatus, time.Time) error {
	return nil
}

func (r *collaborationHandlerRepositoryStub) CreateCommandAndCharge(context.Context, collaborationservice.CreateCommandInput) (collaborationservice.CreateCommandResult, error) {
	return collaborationservice.CreateCommandResult{}, nil
}

func (r *collaborationHandlerRepositoryStub) ExpirePending(context.Context, time.Time) (collaborationservice.SweepResult, error) {
	return collaborationservice.SweepResult{}, nil
}

func TestCollaborationListDevicesUsesJWTSubjectAndHidesInstallationHash(t *testing.T) {
	deviceID := uuid.New()
	repository := &collaborationHandlerRepositoryStub{devices: []collaborationservice.Device{{
		ID:                 deviceID,
		UserID:             42,
		InstallationIDHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Name:               "Workstation",
		Platform:           "linux",
		CompanionVersion:   "0.1.0",
		ProtocolVersion:    1,
		Status:             collaborationdomain.DeviceStatusOffline,
		Capabilities:       map[string]bool{"thread_write": true},
		RegisteredAt:       time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC),
	}}}
	cfg := &config.Config{Collaboration: config.CollaborationConfig{
		ProtocolVersion:   1,
		TaskFeeAmount:     "0.100000",
		TaskFeeCurrency:   "USD",
		CommandTTLSeconds: 300,
		MaxPromptBytes:    32 * 1024,
	}}
	service, err := collaborationservice.NewService(repository, cfg.Collaboration)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	handler := NewCollaborationHandler(service, cfg)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/devices", func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 42})
		c.Next()
	}, handler.ListDevices)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/devices", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if repository.listUserID != 42 {
		t.Fatalf("ListDevices() user ID = %d, want 42", repository.listUserID)
	}
	body := recorder.Body.String()
	if strings.Contains(body, "installation_id_hash") || strings.Contains(body, "aaaaaaaa") || strings.Contains(body, "user_id") {
		t.Fatalf("response exposed private device fields: %s", body)
	}
	if !strings.Contains(body, deviceID.String()) || !strings.Contains(body, "thread_write") {
		t.Fatalf("response omitted public device fields: %s", body)
	}
}
