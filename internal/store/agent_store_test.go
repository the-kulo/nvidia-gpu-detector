package store

import (
	"testing"
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/model"
)

func TestAgentStoreUpdateLastSeenCreatesAgent(t *testing.T) {
	db := setupTestDB(t)
	agentStore := NewAgentStore(db)

	before := time.Now()

	err := agentStore.UpdateLastSeen("agent-001", "test-host", "v0.0.1")
	if err != nil {
		t.Fatalf("update last seen failed: %v", err)
	}

	var got model.Agent
	if err := db.Where("agent_name = ?", "agent-001").First(&got).Error; err != nil {
		t.Fatalf("query agent failed: %v", err)
	}

	if got.AgentName != "agent-001" {
		t.Fatalf("agent_name = %q, want %q", got.AgentName, "agent-001")
	}
	if got.Hostname != "test-host" {
		t.Fatalf("hostname = %q, want %q", got.Hostname, "test-host")
	}
	if got.Version != "v0.0.1" {
		t.Fatalf("version = %q, want %q", got.Version, "v0.0.1")
	}
	if got.Status != model.AgentStatusOnline {
		t.Fatalf("status = %q, want %q", got.Status, model.AgentStatusOnline)
	}
	if got.LastSeenAt.Before(before) {
		t.Fatalf("last_seen_at = %v, want after %v", got.LastSeenAt, before)
	}
}

func TestAgentStoreUpdateLastSeenUpdatesExistingAgent(t *testing.T) {
	db := setupTestDB(t)
	agentStore := NewAgentStore(db)

	oldLastSeenAt := time.Now().Add(-time.Hour)
	agent := model.Agent{
		AgentName:  "agent-001",
		Hostname:   "old-host",
		Version:    "old-version",
		Status:     model.AgentStatusOffline,
		LastSeenAt: oldLastSeenAt,
	}

	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent failed: %v", err)
	}

	err := agentStore.UpdateLastSeen("agent-001", "new-host", "v0.0.2")
	if err != nil {
		t.Fatalf("update last seen failed: %v", err)
	}

	var got model.Agent
	if err := db.Where("agent_name = ?", "agent-001").First(&got).Error; err != nil {
		t.Fatalf("query agent failed: %v", err)
	}

	if got.Hostname != "new-host" {
		t.Fatalf("hostname = %q, want %q", got.Hostname, "new-host")
	}
	if got.Version != "v0.0.2" {
		t.Fatalf("version = %q, want %q", got.Version, "v0.0.2")
	}
	if got.Status != model.AgentStatusOnline {
		t.Fatalf("status = %q, want %q", got.Status, model.AgentStatusOnline)
	}
	if !got.LastSeenAt.After(oldLastSeenAt) {
		t.Fatalf("last_seen_at = %v, want after %v", got.LastSeenAt, oldLastSeenAt)
	}
}

func TestAgentStoreMarkOfflineBefore(t *testing.T) {
	now := time.Now()
	oldLastSeenAt := now.Add(-time.Hour)
	freshLastSeenAt := now
	cutoff := now.Add(-30 * time.Second)

	tests := []struct {
		name          string
		lastSeenAt    time.Time
		initialStatus string
		wantStatus    string
	}{
		{
			name:          "old online agent is marked offline",
			lastSeenAt:    oldLastSeenAt,
			initialStatus: model.AgentStatusOnline,
			wantStatus:    model.AgentStatusOffline,
		},
		{
			name:          "fresh online agent stays online",
			lastSeenAt:    freshLastSeenAt,
			initialStatus: model.AgentStatusOnline,
			wantStatus:    model.AgentStatusOnline,
		},
		{
			name:          "old offline agent stays offline",
			lastSeenAt:    oldLastSeenAt,
			initialStatus: model.AgentStatusOffline,
			wantStatus:    model.AgentStatusOffline,
		},
		{
			name:          "old abnormal agent stays abnormal",
			lastSeenAt:    oldLastSeenAt,
			initialStatus: model.AgentStatusAbnormal,
			wantStatus:    model.AgentStatusAbnormal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			agentStore := NewAgentStore(db)

			agent := model.Agent{
				AgentName:  "agent-001",
				Hostname:   "test-host",
				Version:    "v0.0.1",
				Status:     tt.initialStatus,
				LastSeenAt: tt.lastSeenAt,
			}

			if err := db.Create(&agent).Error; err != nil {
				t.Fatalf("create agent failed: %v", err)
			}

			if err := agentStore.MarkOfflineBefore(cutoff); err != nil {
				t.Fatalf("mark offline before failed: %v", err)
			}

			var got model.Agent
			if err := db.Where("agent_name = ?", agent.AgentName).First(&got).Error; err != nil {
				t.Fatalf("query agent failed: %v", err)
			}

			if got.Status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", got.Status, tt.wantStatus)
			}
		})
	}
}
