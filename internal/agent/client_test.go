package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/heartbeat"
	"github.com/the-kulo/nvidia-gpu-detector/internal/session"
)

func TestRunAgentReregistersAndResetsSessionAfterUnauthorized(t *testing.T) {
	var registerCount atomic.Int32
	heartbeats := make(chan heartbeat.HeartbeatRequest, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/agent/register":
			registration := registerCount.Add(1)
			writeJSON(t, w, session.RegisterResponse{
				SessionID:  "session-" + string(rune('0'+registration)),
				RenewToken: "token-" + string(rune('0'+registration)),
			})
		case "/heartbeat":
			var req heartbeat.HeartbeatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode heartbeat request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			heartbeats <- req
			if req.SessionID == "session-1" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			writeJSON(t, w, heartbeat.HeartbeatResponse{RenewToken: "token-3"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := runAgentAsync(ctx, server.Client(), testConfig(server.URL), time.Millisecond)

	first := receiveHeartbeat(t, heartbeats)
	second := receiveHeartbeat(t, heartbeats)
	cancel()
	assertAgentStopped(t, done)

	if got := registerCount.Load(); got != 2 {
		t.Fatalf("registration count = %d, want 2", got)
	}
	if first.SessionID != "session-1" || first.RenewToken != "token-1" || first.Sequence != 1 {
		t.Fatalf("first heartbeat session = (%q, %q, %d), want (%q, %q, 1)",
			first.SessionID, first.RenewToken, first.Sequence, "session-1", "token-1")
	}
	if second.SessionID != "session-2" || second.RenewToken != "token-2" || second.Sequence != 1 {
		t.Fatalf("second heartbeat session = (%q, %q, %d), want (%q, %q, 1)",
			second.SessionID, second.RenewToken, second.Sequence, "session-2", "token-2")
	}
}

func TestRunAgentDoesNotReregisterAfterServerError(t *testing.T) {
	var registerCount atomic.Int32
	heartbeats := make(chan heartbeat.HeartbeatRequest, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/agent/register":
			registerCount.Add(1)
			writeJSON(t, w, session.RegisterResponse{
				SessionID:  "session-1",
				RenewToken: "token-1",
			})
		case "/heartbeat":
			var req heartbeat.HeartbeatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode heartbeat request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			heartbeats <- req
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := runAgentAsync(ctx, server.Client(), testConfig(server.URL), time.Millisecond)

	first := receiveHeartbeat(t, heartbeats)
	second := receiveHeartbeat(t, heartbeats)
	cancel()
	assertAgentStopped(t, done)

	if got := registerCount.Load(); got != 1 {
		t.Fatalf("registration count = %d, want 1", got)
	}
	if first.SessionID != "session-1" || first.RenewToken != "token-1" {
		t.Fatalf("first heartbeat used unexpected credentials: session=%q token=%q", first.SessionID, first.RenewToken)
	}
	if second.SessionID != "session-1" || second.RenewToken != "token-1" {
		t.Fatalf("retry used unexpected credentials: session=%q token=%q", second.SessionID, second.RenewToken)
	}
}

func TestSendHeartbeatReturnsHTTPStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := sendHeartbeat(context.Background(), server.Client(), server.URL, heartbeat.HeartbeatRequest{})
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error = %v, want *HTTPStatusError", err)
	}
	if statusErr.Operation != "heartbeat" || statusErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status error = %#v, want heartbeat status 401", statusErr)
	}
}

func runAgentAsync(ctx context.Context, client *http.Client, cfg Config, retryInterval time.Duration) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- runAgent(ctx, client, cfg, retryInterval)
	}()
	return done
}

func testConfig(centerURL string) Config {
	return Config{
		AgentName: "agent-001",
		Hostname:  "host-001",
		Version:   "test",
		CenterURL: centerURL,
	}
}

func receiveHeartbeat(t *testing.T, requests <-chan heartbeat.HeartbeatRequest) heartbeat.HeartbeatRequest {
	t.Helper()
	select {
	case req := <-requests:
		return req
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for heartbeat")
		return heartbeat.HeartbeatRequest{}
	}
}

func assertAgentStopped(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runAgent error = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runAgent did not stop after context cancellation")
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}
