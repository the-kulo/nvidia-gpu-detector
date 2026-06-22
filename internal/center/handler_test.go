package center

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/heartbeat"
	"github.com/the-kulo/nvidia-gpu-detector/internal/model"
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

func setupTestServer(t *testing.T) testServer {
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
	testServer := setupTestServer(t)

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

func TestHeartbeatHandlerBusinessRules(t *testing.T) {
	testServer := setupTestServer(t)

	firstRegisterResp := registerAgent(t, testServer.handler, "agent-001")

	status := postJSON(t, testServer.handler, "/heartbeat", heartbeat.HeartbeatRequest{
		AgentName:  "agent-001",
		SessionID:  firstRegisterResp.SessionID,
		Hostname:   "test-host",
		Version:    "v0.0.1",
		Sequence:   1,
		RenewToken: "wrong-token",
	}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong token status = %d, want %d", status, http.StatusUnauthorized)
	}

	firstHeartbeatResp := sendHeartbeat(
		t,
		testServer.handler,
		"agent-001",
		firstRegisterResp.SessionID,
		firstRegisterResp.RenewToken,
		1,
	)
	if firstHeartbeatResp.RenewToken == "" {
		t.Fatal("heartbeat renew_token is empty")
	}
	if firstHeartbeatResp.RenewToken == firstRegisterResp.RenewToken {
		t.Fatal("heartbeat did not rotate renew token")
	}

	status = postJSON(t, testServer.handler, "/heartbeat", heartbeat.HeartbeatRequest{
		AgentName:  "agent-001",
		SessionID:  firstRegisterResp.SessionID,
		Hostname:   "test-host",
		Version:    "v0.0.1",
		Sequence:   1,
		RenewToken: firstHeartbeatResp.RenewToken,
	}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("repeated sequence status = %d, want %d", status, http.StatusUnauthorized)
	}

	secondRegisterResp := registerAgent(t, testServer.handler, "agent-001")
	if secondRegisterResp.SessionID == firstRegisterResp.SessionID {
		t.Fatal("new register reused old session_id")
	}

	_ = sendHeartbeat(
		t,
		testServer.handler,
		"agent-001",
		secondRegisterResp.SessionID,
		secondRegisterResp.RenewToken,
		1,
	)

	oldLastSeenAt := time.Now().Add(-time.Hour)
	if err := testServer.db.Model(&model.AgentSession{}).
		Where("session_id = ?", firstRegisterResp.SessionID).
		Update("last_seen_at", oldLastSeenAt).
		Error; err != nil {
		t.Fatalf("age old session failed: %v", err)
	}

	sessionStore := store.NewSessionStore(testServer.db)
	if err := sessionStore.MarkExpiredBefore(time.Now().Add(-30 * time.Second)); err != nil {
		t.Fatalf("mark expired before failed: %v", err)
	}

	var oldSession model.AgentSession
	if err := testServer.db.Where("session_id = ?", firstRegisterResp.SessionID).First(&oldSession).Error; err != nil {
		t.Fatalf("query old session failed: %v", err)
	}
	if oldSession.Status != model.SessionStatusEnded {
		t.Fatalf("old session status = %q, want %q", oldSession.Status, model.SessionStatusEnded)
	}

	var newSession model.AgentSession
	if err := testServer.db.Where("session_id = ?", secondRegisterResp.SessionID).First(&newSession).Error; err != nil {
		t.Fatalf("query new session failed: %v", err)
	}
	if newSession.Status != model.SessionStatusOnline {
		t.Fatalf("new session status = %q, want %q", newSession.Status, model.SessionStatusOnline)
	}

	status = postJSON(t, testServer.handler, "/heartbeat", heartbeat.HeartbeatRequest{
		AgentName:  "agent-001",
		SessionID:  firstRegisterResp.SessionID,
		Hostname:   "test-host",
		Version:    "v0.0.1",
		Sequence:   2,
		RenewToken: firstHeartbeatResp.RenewToken,
	}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("ended old session heartbeat status = %d, want %d", status, http.StatusUnauthorized)
	}
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

func sendHeartbeat(
	t *testing.T,
	handler http.Handler,
	agentName string,
	sessionID string,
	renewToken string,
	sequence int64,
) heartbeat.HeartbeatResponse {
	t.Helper()

	var resp heartbeat.HeartbeatResponse
	status := postJSON(t, handler, "/heartbeat", heartbeat.HeartbeatRequest{
		AgentName:  agentName,
		SessionID:  sessionID,
		Hostname:   "test-host",
		Version:    "v0.0.1",
		Sequence:   sequence,
		RenewToken: renewToken,
	}, &resp)
	if status != http.StatusOK {
		t.Fatalf("heartbeat status = %d, want %d", status, http.StatusOK)
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
