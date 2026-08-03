package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	collaborationdomain "github.com/Wei-Shaw/sub2api/internal/domain/collaboration"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	collaborationservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type collaborationHandlerRepositoryStub struct {
	listUserID   int64
	devices      []collaborationservice.Device
	device       collaborationservice.Device
	command      collaborationservice.Command
	commandInput *collaborationservice.CreateCommandInput
	statuses     chan collaborationdomain.DeviceStatus
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
	return r.device, nil
}

func (r *collaborationHandlerRepositoryStub) UpdateDevicePresence(_ context.Context, _ int64, _ uuid.UUID, status collaborationdomain.DeviceStatus, _ time.Time) error {
	if r.statuses != nil {
		r.statuses <- status
	}
	return nil
}

func (r *collaborationHandlerRepositoryStub) CreateSync(context.Context, collaborationservice.CreateSyncInput) (collaborationservice.CreateSyncResult, error) {
	return collaborationservice.CreateSyncResult{}, nil
}

func (r *collaborationHandlerRepositoryStub) GetSync(context.Context, int64, uuid.UUID) (collaborationservice.SyncRequest, error) {
	return collaborationservice.SyncRequest{}, nil
}

func (r *collaborationHandlerRepositoryStub) TransitionSync(context.Context, collaborationservice.SyncTransitionInput) (collaborationservice.SyncRequest, error) {
	return collaborationservice.SyncRequest{}, nil
}

func (r *collaborationHandlerRepositoryStub) CreateCommandAndCharge(_ context.Context, input collaborationservice.CreateCommandInput) (collaborationservice.CreateCommandResult, error) {
	r.commandInput = &input
	if r.command.ID == uuid.Nil {
		r.command = collaborationservice.Command{
			ID: input.IdempotencyKey, UserID: input.UserID, DeviceID: input.DeviceID,
			ThreadID: input.ThreadID, IdempotencyKey: input.IdempotencyKey,
			Status: collaborationdomain.CommandStatusAccepted, ExpiresAt: input.ExpiresAt,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
	}
	return collaborationservice.CreateCommandResult{Command: r.command}, nil
}

func (r *collaborationHandlerRepositoryStub) GetCommand(context.Context, int64, uuid.UUID) (collaborationservice.Command, error) {
	return r.command, nil
}

func (r *collaborationHandlerRepositoryStub) TransitionCommand(_ context.Context, input collaborationservice.CommandTransitionInput) (collaborationservice.Command, error) {
	r.command.Status = input.Status
	r.command.TurnID = input.TurnID
	r.command.ErrorCode = input.ErrorCode
	r.command.UpdatedAt = input.OccurredAt
	return r.command, nil
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
	handler := NewCollaborationHandler(service, cfg, nil, nil)

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

func TestCollaborationCommandDTOOmitsBackendBillingState(t *testing.T) {
	dto := collaborationCommandDTO(collaborationservice.Command{
		ID: uuid.New(), Status: collaborationdomain.CommandStatusDispatched,
	})
	for _, key := range []string{"charge", "fee", "amount", "currency", "balance", "balance_after"} {
		if _, exposed := dto[key]; exposed {
			t.Fatalf("command response exposed backend billing field %q", key)
		}
	}
}

func TestCollaborationCreateCommandDispatchesWithoutExposingBackendFee(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deviceID := uuid.New()
	idempotencyKey := uuid.New()
	repository := &collaborationHandlerRepositoryStub{device: collaborationservice.Device{
		ID: deviceID, UserID: 42, ProtocolVersion: 1,
		Capabilities: map[string]bool{"thread_write": true},
	}}
	presence := &collaborationHandlerPresenceStub{items: map[uuid.UUID]collaborationservice.DevicePresence{
		deviceID: {
			DeviceID: deviceID, UserID: 42, Status: collaborationdomain.DeviceStatusOnline,
			AppServerStatus: "ready", LastSeenAt: time.Now().UTC(),
		},
	}}
	eventBus := &collaborationHandlerEventBusStub{
		deviceEvents: make(chan collaborationservice.EventEnvelope, 1),
		userEvents:   make(chan collaborationservice.EventEnvelope, 1),
	}
	payloads := &collaborationHandlerPayloadStoreStub{}
	cfg := &config.Config{Collaboration: config.CollaborationConfig{
		ProtocolVersion: 1, TaskFeeAmount: "0.100000", TaskFeeCurrency: "USD",
		CommandTTLSeconds: 300, MaxPromptBytes: 32 * 1024,
	}}
	service, err := collaborationservice.NewService(repository, cfg.Collaboration)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.SetPresenceStore(presence)
	service.SetRealtime(eventBus, payloads)
	handler := NewCollaborationHandler(service, cfg, eventBus, collaborationHandlerConnectionLeaseStub{})
	router := gin.New()
	router.POST("/commands", func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 42})
		c.Next()
	}, handler.CreateCommand)
	body := `{"device_id":"` + deviceID.String() + `","thread_id":"thread_123","input":[{"type":"text","text":"继续任务"}],"client_context":{"source":"android"}}`
	request := httptest.NewRequest(http.MethodPost, "/commands", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey.String())
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	responseBody := recorder.Body.String()
	for _, forbidden := range []string{"0.100000", "USD", "charge", "balance_after", "fee"} {
		if strings.Contains(responseBody, forbidden) {
			t.Fatalf("response exposed backend billing data %q: %s", forbidden, responseBody)
		}
	}
	if repository.commandInput == nil || repository.commandInput.Fee.StringFixed(6) != "0.100000" {
		t.Fatalf("repository did not receive server-owned fee: %#v", repository.commandInput)
	}
	if payloads.prompt != "继续任务" || repository.command.Status != collaborationdomain.CommandStatusDispatched {
		t.Fatalf("prompt/status = %q/%s", payloads.prompt, repository.command.Status)
	}
}

type collaborationHandlerPresenceStub struct {
	touched chan collaborationservice.DevicePresence
	items   map[uuid.UUID]collaborationservice.DevicePresence
}

func (s *collaborationHandlerPresenceStub) Touch(_ context.Context, presence collaborationservice.DevicePresence) error {
	if s.touched != nil {
		s.touched <- presence
	}
	return nil
}

func (s *collaborationHandlerPresenceStub) GetMany(context.Context, []uuid.UUID) (map[uuid.UUID]collaborationservice.DevicePresence, error) {
	return s.items, nil
}

func (s *collaborationHandlerPresenceStub) Remove(context.Context, uuid.UUID) error {
	return nil
}

type collaborationHandlerSubscriptionStub struct {
	events chan collaborationservice.EventEnvelope
}

func (s *collaborationHandlerSubscriptionStub) Events() <-chan collaborationservice.EventEnvelope {
	return s.events
}

func (s *collaborationHandlerSubscriptionStub) Close() error {
	return nil
}

type collaborationHandlerEventBusStub struct {
	deviceEvents chan collaborationservice.EventEnvelope
	userEvents   chan collaborationservice.EventEnvelope
	sequence     atomic.Int64
}

type collaborationHandlerPayloadStoreStub struct {
	prompt string
}

func (s *collaborationHandlerPayloadStoreStub) PutCommand(_ context.Context, _ int64, _ uuid.UUID, prompt string) error {
	s.prompt = prompt
	return nil
}

func (s *collaborationHandlerPayloadStoreStub) GetCommand(context.Context, int64, uuid.UUID) (string, error) {
	return s.prompt, nil
}

func (s *collaborationHandlerPayloadStoreStub) PutSync(context.Context, int64, uuid.UUID, collaborationdomain.SyncKind, []byte) error {
	return nil
}

func (s *collaborationHandlerPayloadStoreStub) GetSync(context.Context, int64, uuid.UUID, collaborationdomain.SyncKind) ([]byte, error) {
	return nil, nil
}

func (s *collaborationHandlerPayloadStoreStub) DeleteSync(context.Context, int64, uuid.UUID, collaborationdomain.SyncKind) error {
	return nil
}

type collaborationHandlerConnectionLeaseStub struct {
	deny bool
}

func (s collaborationHandlerConnectionLeaseStub) Acquire(context.Context, collaborationservice.ConnectionLease) (bool, error) {
	return !s.deny, nil
}

func (collaborationHandlerConnectionLeaseStub) Renew(context.Context, collaborationservice.ConnectionLease) (bool, error) {
	return true, nil
}

func (collaborationHandlerConnectionLeaseStub) Release(context.Context, collaborationservice.ConnectionLease) error {
	return nil
}

func (b *collaborationHandlerEventBusStub) PublishUser(_ context.Context, _ int64, eventType string, requestID *string, payload map[string]any) (collaborationservice.EventEnvelope, error) {
	event := b.event(eventType, requestID, payload)
	b.userEvents <- event
	return event, nil
}

func (b *collaborationHandlerEventBusStub) PublishDevice(_ context.Context, _ int64, _ uuid.UUID, eventType string, requestID *string, payload map[string]any) (collaborationservice.EventEnvelope, error) {
	event := b.event(eventType, requestID, payload)
	b.deviceEvents <- event
	return event, nil
}

func (b *collaborationHandlerEventBusStub) SubscribeUser(context.Context, int64) (collaborationservice.EventSubscription, error) {
	return &collaborationHandlerSubscriptionStub{events: b.userEvents}, nil
}

func (b *collaborationHandlerEventBusStub) SubscribeDevice(context.Context, uuid.UUID) (collaborationservice.EventSubscription, error) {
	return &collaborationHandlerSubscriptionStub{events: b.deviceEvents}, nil
}

func (b *collaborationHandlerEventBusStub) event(eventType string, requestID *string, payload map[string]any) collaborationservice.EventEnvelope {
	return collaborationservice.EventEnvelope{
		Version:    1,
		Type:       eventType,
		EventID:    uuid.NewString(),
		RequestID:  requestID,
		Sequence:   b.sequence.Add(1),
		OccurredAt: time.Now().UTC(),
		Payload:    payload,
	}
}

func TestCollaborationWebSocketHeartbeatRefreshesPresence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deviceID := uuid.New()
	repository := &collaborationHandlerRepositoryStub{
		device: collaborationservice.Device{
			ID:              deviceID,
			UserID:          42,
			ProtocolVersion: 1,
			Status:          collaborationdomain.DeviceStatusOffline,
		},
		statuses: make(chan collaborationdomain.DeviceStatus, 2),
	}
	presence := &collaborationHandlerPresenceStub{touched: make(chan collaborationservice.DevicePresence, 1)}
	eventBus := &collaborationHandlerEventBusStub{
		deviceEvents: make(chan collaborationservice.EventEnvelope, 4),
		userEvents:   make(chan collaborationservice.EventEnvelope, 4),
	}
	cfg := &config.Config{Collaboration: config.CollaborationConfig{
		ProtocolVersion:    1,
		PresenceTTLSeconds: 45,
		TaskFeeAmount:      "0.100000",
		TaskFeeCurrency:    "USD",
		CommandTTLSeconds:  300,
		MaxPromptBytes:     32 * 1024,
		MaxEventBytes:      1024 * 1024,
	}}
	service, err := collaborationservice.NewService(repository, cfg.Collaboration)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.SetPresenceStore(presence)
	handler := NewCollaborationHandler(service, cfg, eventBus, collaborationHandlerConnectionLeaseStub{})
	router := gin.New()
	router.GET("/ws", func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 42})
		c.Next()
	}, handler.WebSocket)
	server := httptest.NewServer(router)
	defer server.Close()

	headers := http.Header{}
	headers.Set("X-Sub2API-Client-Type", "pc")
	headers.Set("X-Sub2API-Device-ID", deviceID.String())
	headers.Set("X-Sub2API-Protocol-Version", "1")
	connection, _, err := websocket.DefaultDialer.Dial(strings.Replace(server.URL, "http://", "ws://", 1)+"/ws", headers)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer func() { _ = connection.Close() }()
	requestID := uuid.NewString()
	if err := connection.WriteJSON(collaborationservice.EventEnvelope{
		Version:    1,
		Type:       "heartbeat",
		EventID:    uuid.NewString(),
		RequestID:  &requestID,
		Sequence:   0,
		OccurredAt: time.Now().UTC(),
		Payload: map[string]any{
			"app_server_status":   "ready",
			"active_thread_count": 2,
		},
	}); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}

	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	var acknowledgement collaborationservice.EventEnvelope
	if err := connection.ReadJSON(&acknowledgement); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}
	if acknowledgement.Type != "heartbeat.ack" || acknowledgement.RequestID == nil || *acknowledgement.RequestID != requestID {
		t.Fatalf("acknowledgement = %#v", acknowledgement)
	}
	select {
	case touched := <-presence.touched:
		if touched.DeviceID != deviceID || touched.Status != collaborationdomain.DeviceStatusOnline || touched.ActiveThreadCount != 2 {
			t.Fatalf("presence = %#v", touched)
		}
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not refresh presence")
	}
	select {
	case event := <-eventBus.userEvents:
		if event.Type != "device.presence_changed" || event.Payload["status"] != collaborationdomain.DeviceStatusOnline {
			t.Fatalf("user event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not publish user presence event")
	}
}

func TestCollaborationWebSocketOriginPolicy(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://panel.example/ws", nil)
	request.Host = "panel.example"
	if !collaborationWebSocketOriginAllowed(request) {
		t.Fatal("native client without Origin was rejected")
	}
	request.Header.Set("Origin", "https://panel.example")
	if !collaborationWebSocketOriginAllowed(request) {
		t.Fatal("same-origin browser client was rejected")
	}
	request.Header.Set("Origin", "https://evil.example")
	if collaborationWebSocketOriginAllowed(request) {
		t.Fatal("cross-origin browser client was accepted")
	}
}

func TestCollaborationWebSocketRejectsConnectionOverLimitBeforeUpgrade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Collaboration: config.CollaborationConfig{
		ProtocolVersion:   1,
		TaskFeeAmount:     "0.100000",
		TaskFeeCurrency:   "USD",
		CommandTTLSeconds: 300,
		MaxPromptBytes:    32 * 1024,
	}}
	eventBus := &collaborationHandlerEventBusStub{
		deviceEvents: make(chan collaborationservice.EventEnvelope, 1),
		userEvents:   make(chan collaborationservice.EventEnvelope, 1),
	}
	handler := NewCollaborationHandler(nil, cfg, eventBus, collaborationHandlerConnectionLeaseStub{deny: true})
	router := gin.New()
	router.GET("/ws", func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 42})
		c.Next()
	}, handler.WebSocket)
	request := httptest.NewRequest(http.MethodGet, "/ws", nil)
	request.Header.Set("X-Sub2API-Client-Type", "mobile")
	request.Header.Set("X-Sub2API-Protocol-Version", "1")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusTooManyRequests || !strings.Contains(recorder.Body.String(), "COLLAB_CONNECTION_LIMIT") {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
