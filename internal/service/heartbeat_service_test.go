package service

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

var testCommand = HeartbeatCommand{
	AgentName:  "agent-001",
	SessionID:  "session-001",
	Hostname:   "host-001",
	Version:    "v1.2.3",
	Sequence:   7,
	RenewToken: "old-token",
}

type acceptHeartbeatCall struct {
	agentName         string
	sessionID         string
	sequence          int64
	currentRenewToken string
	newRenewToken     string
}

type fakeSessionStore struct {
	calls []acceptHeartbeatCall
	err   error
}

func (s *fakeSessionStore) AcceptHeartbeat(
	agentName string,
	sessionID string,
	sequence int64,
	currentRenewToken string,
	newRenewToken string,
) error {
	s.calls = append(s.calls, acceptHeartbeatCall{
		agentName:         agentName,
		sessionID:         sessionID,
		sequence:          sequence,
		currentRenewToken: currentRenewToken,
		newRenewToken:     newRenewToken,
	})
	return s.err
}

type updateLastSeenCall struct {
	agentName string
	hostname  string
	version   string
}

type fakeAgentStore struct {
	calls []updateLastSeenCall
	err   error
}

func (s *fakeAgentStore) UpdateLastSeen(agentName string, hostname string, version string) error {
	s.calls = append(s.calls, updateLastSeenCall{
		agentName: agentName,
		hostname:  hostname,
		version:   version,
	})
	return s.err
}

func TestHeartbeatServiceHandleHeartbeatSuccess(t *testing.T) {
	agentStore := &fakeAgentStore{}
	sessionStore := &fakeSessionStore{}
	serverTime := time.Date(2026, time.July, 13, 10, 30, 0, 0, time.UTC)
	service := newHeartbeatService(
		agentStore,
		sessionStore,
		func() (string, error) { return "new-token", nil },
		func() time.Time { return serverTime },
	)

	result, err := service.HandleHeartbeat(testCommand)
	if err != nil {
		t.Fatalf("HandleHeartbeat returned error: %v", err)
	}

	wantResult := HeartbeatResult{
		ServerTime:       serverTime,
		RenewToken:       "new-token",
		NextHeartbeatSec: 10,
	}
	if !reflect.DeepEqual(result, wantResult) {
		t.Fatalf("unexpected result: got %+v, want %+v", result, wantResult)
	}

	wantSessionCalls := []acceptHeartbeatCall{{
		agentName:         testCommand.AgentName,
		sessionID:         testCommand.SessionID,
		sequence:          testCommand.Sequence,
		currentRenewToken: testCommand.RenewToken,
		newRenewToken:     "new-token",
	}}
	if !reflect.DeepEqual(sessionStore.calls, wantSessionCalls) {
		t.Fatalf("unexpected session store calls: got %+v, want %+v", sessionStore.calls, wantSessionCalls)
	}

	wantAgentCalls := []updateLastSeenCall{{
		agentName: testCommand.AgentName,
		hostname:  testCommand.Hostname,
		version:   testCommand.Version,
	}}
	if !reflect.DeepEqual(agentStore.calls, wantAgentCalls) {
		t.Fatalf("unexpected agent store calls: got %+v, want %+v", agentStore.calls, wantAgentCalls)
	}
}

func TestHeartbeatServiceHandleHeartbeatTokenGenerationFailure(t *testing.T) {
	tokenErr := errors.New("random source unavailable")
	agentStore := &fakeAgentStore{}
	sessionStore := &fakeSessionStore{}
	service := newHeartbeatService(
		agentStore,
		sessionStore,
		func() (string, error) { return "", tokenErr },
		time.Now,
	)

	result, err := service.HandleHeartbeat(testCommand)
	assertHeartbeatError(t, result, err, tokenErr, "generate heartbeat renew token")
	assertNoStoreCalls(t, agentStore, sessionStore)
}

func TestHeartbeatServiceHandleHeartbeatRejected(t *testing.T) {
	agentStore := &fakeAgentStore{}
	sessionStore := &fakeSessionStore{err: ErrHeartbeatRejected}
	service := newHeartbeatService(
		agentStore,
		sessionStore,
		func() (string, error) { return "new-token", nil },
		time.Now,
	)

	result, err := service.HandleHeartbeat(testCommand)
	assertHeartbeatError(t, result, err, ErrHeartbeatRejected, "accept heartbeat")
	if len(sessionStore.calls) != 1 {
		t.Fatalf("AcceptHeartbeat call count: got %d, want 1", len(sessionStore.calls))
	}
	if len(agentStore.calls) != 0 {
		t.Fatalf("UpdateLastSeen call count: got %d, want 0", len(agentStore.calls))
	}
}

func TestHeartbeatServiceHandleHeartbeatSessionStoreFailure(t *testing.T) {
	storeErr := errors.New("database unavailable")
	agentStore := &fakeAgentStore{}
	sessionStore := &fakeSessionStore{err: storeErr}
	service := newHeartbeatService(
		agentStore,
		sessionStore,
		func() (string, error) { return "new-token", nil },
		time.Now,
	)

	result, err := service.HandleHeartbeat(testCommand)
	assertHeartbeatError(t, result, err, storeErr, "accept heartbeat")
	if len(sessionStore.calls) != 1 {
		t.Fatalf("AcceptHeartbeat call count: got %d, want 1", len(sessionStore.calls))
	}
	if len(agentStore.calls) != 0 {
		t.Fatalf("UpdateLastSeen call count: got %d, want 0", len(agentStore.calls))
	}
}

func TestHeartbeatServiceHandleHeartbeatAgentStoreFailure(t *testing.T) {
	storeErr := errors.New("agent database unavailable")
	agentStore := &fakeAgentStore{err: storeErr}
	sessionStore := &fakeSessionStore{}
	service := newHeartbeatService(
		agentStore,
		sessionStore,
		func() (string, error) { return "new-token", nil },
		time.Now,
	)

	result, err := service.HandleHeartbeat(testCommand)
	assertHeartbeatError(t, result, err, storeErr, "update agent last seen")
	if len(sessionStore.calls) != 1 {
		t.Fatalf("AcceptHeartbeat call count: got %d, want 1", len(sessionStore.calls))
	}
	if len(agentStore.calls) != 1 {
		t.Fatalf("UpdateLastSeen call count: got %d, want 1", len(agentStore.calls))
	}
}

func assertHeartbeatError(
	t *testing.T,
	result HeartbeatResult,
	err error,
	wantError error,
	wantContext string,
) {
	t.Helper()
	if err == nil {
		t.Fatal("HandleHeartbeat returned nil error")
	}
	if !errors.Is(err, wantError) {
		t.Fatalf("error does not wrap expected error: got %v, want %v", err, wantError)
	}
	if !strings.Contains(err.Error(), wantContext) {
		t.Fatalf("error lacks context %q: %v", wantContext, err)
	}
	if result != (HeartbeatResult{}) {
		t.Fatalf("unexpected result on failure: got %+v, want zero value", result)
	}
}

func assertNoStoreCalls(t *testing.T, agentStore *fakeAgentStore, sessionStore *fakeSessionStore) {
	t.Helper()
	if len(sessionStore.calls) != 0 {
		t.Fatalf("AcceptHeartbeat call count: got %d, want 0", len(sessionStore.calls))
	}
	if len(agentStore.calls) != 0 {
		t.Fatalf("UpdateLastSeen call count: got %d, want 0", len(agentStore.calls))
	}
}
