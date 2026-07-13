package center

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/heartbeat"
	"github.com/the-kulo/nvidia-gpu-detector/internal/model"
	"github.com/the-kulo/nvidia-gpu-detector/internal/service"
	"github.com/the-kulo/nvidia-gpu-detector/internal/session"
	"github.com/the-kulo/nvidia-gpu-detector/internal/store"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testServer struct {
	handler http.Handler
	db      *gorm.DB
}

type fakeHeartbeatService struct {
	commands []service.HeartbeatCommand
	result   service.HeartbeatResult
	err      error
}

func (s *fakeHeartbeatService) HandleHeartbeat(command service.HeartbeatCommand) (service.HeartbeatResult, error) {
	s.commands = append(s.commands, command)
	return s.result, s.err
}

func setupTestServer(t *testing.T, heartbeatService service.HeartbeatService) testServer {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open test db failed: %v", err)
	}

	if err := db.AutoMigrate(&model.Agent{}, &model.AgentSession{}); err != nil {
		t.Fatalf("migrate test db failed: %v", err)
	}

	server := NewServer(
		store.NewAgentStore(db),
		store.NewSessionStore(db),
		heartbeatService,
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/agent/register", server.RegisterHandler)
	mux.HandleFunc("/heartbeat", server.HeartbeatHandler)

	return testServer{
		handler: mux,
		db:      db,
	}
}

func TestRegisterHandlerCreatesAgentAndSession(t *testing.T) {
	testServer := setupTestServer(t, &fakeHeartbeatService{})

	registerResp := registerAgent(t, testServer.handler, "agent-001")

	if registerResp.SessionID == "" {
		t.Fatal("session_id is empty")
	}
	if registerResp.RenewToken == "" {
		t.Fatal("renew_token is empty")
	}

	var agent model.Agent
	if err := testServer.db.Where("agent_name = ?", "agent-001").First(&agent).Error; err != nil {
		t.Fatalf("query agent failed: %v", err)
	}
	if agent.Status != model.AgentStatusOnline {
		t.Fatalf("agent status = %q, want %q", agent.Status, model.AgentStatusOnline)
	}

	var agentSession model.AgentSession
	if err := testServer.db.Where("session_id = ?", registerResp.SessionID).First(&agentSession).Error; err != nil {
		t.Fatalf("query session failed: %v", err)
	}
	if agentSession.AgentName != "agent-001" {
		t.Fatalf("session agent_name = %q, want %q", agentSession.AgentName, "agent-001")
	}
	if agentSession.Status != model.SessionStatusOnline {
		t.Fatalf("session status = %q, want %q", agentSession.Status, model.SessionStatusOnline)
	}
	if agentSession.LastSequence != 0 {
		t.Fatalf("last_sequence = %d, want 0", agentSession.LastSequence)
	}
}

func TestHeartbeatHandlerMapsCommandAndResult(t *testing.T) {
	serverTime := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	heartbeatService := &fakeHeartbeatService{result: service.HeartbeatResult{
		ServerTime:       serverTime,
		RenewToken:       "new-token",
		NextHeartbeatSec: 15,
	}}
	testServer := setupTestServer(t, heartbeatService)

	request := heartbeat.HeartbeatRequest{
		AgentName:  "agent-001",
		SessionID:  "session-001",
		Hostname:   "test-host",
		Version:    "v0.0.1",
		Sequence:   4,
		RenewToken: "old-token",
	}
	var response heartbeat.HeartbeatResponse
	status := postJSON(t, testServer.handler, "/heartbeat", request, &response)

	if status != http.StatusOK {
		t.Fatalf("heartbeat status = %d, want %d", status, http.StatusOK)
	}
	wantCommand := service.HeartbeatCommand{
		AgentName:  request.AgentName,
		SessionID:  request.SessionID,
		Hostname:   request.Hostname,
		Version:    request.Version,
		Sequence:   request.Sequence,
		RenewToken: request.RenewToken,
	}
	if !reflect.DeepEqual(heartbeatService.commands, []service.HeartbeatCommand{wantCommand}) {
		t.Fatalf("service commands = %+v, want %+v", heartbeatService.commands, []service.HeartbeatCommand{wantCommand})
	}
	wantResponse := heartbeat.HeartbeatResponse{
		ServerTime:       serverTime,
		RenewToken:       "new-token",
		NextHeartbeatSec: 15,
	}
	if !reflect.DeepEqual(response, wantResponse) {
		t.Fatalf("heartbeat response = %+v, want %+v", response, wantResponse)
	}
}

func TestHeartbeatHandlerMapsRejectedToUnauthorized(t *testing.T) {
	heartbeatService := &fakeHeartbeatService{err: service.ErrHeartbeatRejected}
	testServer := setupTestServer(t, heartbeatService)

	status := postJSON(t, testServer.handler, "/heartbeat", validHeartbeatRequest(), nil)

	if status != http.StatusUnauthorized {
		t.Fatalf("heartbeat status = %d, want %d", status, http.StatusUnauthorized)
	}
	if len(heartbeatService.commands) != 1 {
		t.Fatalf("service call count = %d, want 1", len(heartbeatService.commands))
	}
}

func TestHeartbeatHandlerMapsOperationalErrorToInternalServerError(t *testing.T) {
	heartbeatService := &fakeHeartbeatService{err: errors.New("database unavailable")}
	testServer := setupTestServer(t, heartbeatService)

	status := postJSON(t, testServer.handler, "/heartbeat", validHeartbeatRequest(), nil)

	if status != http.StatusInternalServerError {
		t.Fatalf("heartbeat status = %d, want %d", status, http.StatusInternalServerError)
	}
	if len(heartbeatService.commands) != 1 {
		t.Fatalf("service call count = %d, want 1", len(heartbeatService.commands))
	}
}

func TestHeartbeatHandlerDoesNotCallServiceForInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{name: "malformed JSON", body: []byte("{")},
		{name: "missing agent name", body: mustMarshal(t, heartbeat.HeartbeatRequest{SessionID: "session-001", Sequence: 1})},
		{name: "missing session ID", body: mustMarshal(t, heartbeat.HeartbeatRequest{AgentName: "agent-001", Sequence: 1})},
		{name: "invalid sequence", body: mustMarshal(t, heartbeat.HeartbeatRequest{AgentName: "agent-001", SessionID: "session-001", Sequence: 0})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			heartbeatService := &fakeHeartbeatService{}
			testServer := setupTestServer(t, heartbeatService)
			request := httptest.NewRequest(http.MethodPost, "/heartbeat", bytes.NewReader(tt.body))
			recorder := httptest.NewRecorder()

			testServer.handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("heartbeat status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			if len(heartbeatService.commands) != 0 {
				t.Fatalf("service call count = %d, want 0", len(heartbeatService.commands))
			}
		})
	}
}

func validHeartbeatRequest() heartbeat.HeartbeatRequest {
	return heartbeat.HeartbeatRequest{
		AgentName:  "agent-001",
		SessionID:  "session-001",
		Hostname:   "test-host",
		Version:    "v0.0.1",
		Sequence:   1,
		RenewToken: "old-token",
	}
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}
	return body
}

func registerAgent(t *testing.T, handler http.Handler, agentName string) session.RegisterResponse {
	t.Helper()

	var resp session.RegisterResponse
	status := postJSON(t, handler, "/agent/register", session.RegisterRequest{
		AgentName: agentName,
		Hostname:  "test-host",
		Version:   "v0.0.1",
	}, &resp)
	if status != http.StatusOK {
		t.Fatalf("register status = %d, want %d", status, http.StatusOK)
	}

	return resp
}

func postJSON(t *testing.T, handler http.Handler, path string, req any, resp any) int {
	t.Helper()

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if resp != nil && recorder.Code == http.StatusOK {
		if err := json.NewDecoder(recorder.Body).Decode(resp); err != nil {
			t.Fatalf("decode response failed: %v", err)
		}
	}

	return recorder.Code
}
