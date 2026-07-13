package store

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/heartbeat"
	"github.com/the-kulo/nvidia-gpu-detector/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
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

	return db
}

func TestSessionStoreVerifyHeartbeat(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		session    *model.AgentSession
		agentName  string
		sessionID  string
		sequence   int64
		renewToken string
		wantErr    bool
	}{
		{
			name: "valid next sequence and token",
			session: &model.AgentSession{
				AgentName:      "agent-001",
				SessionID:      "session-001",
				Hostname:       "LAPTOP-TEST",
				Version:        "v0.0.1",
				Status:         model.SessionStatusOnline,
				LastSequence:   10,
				LastRenewToken: "token-test",
				LastSeenAt:     now,
				StartedAt:      now,
			},
			agentName:  "agent-001",
			sessionID:  "session-001",
			sequence:   11,
			renewToken: "token-test",
			wantErr:    false,
		},
		{
			name: "wrong token is rejected",
			session: &model.AgentSession{
				AgentName:      "agent-001",
				SessionID:      "session-001",
				Status:         model.SessionStatusOnline,
				LastSequence:   10,
				LastRenewToken: "token-test",
				LastSeenAt:     now,
				StartedAt:      now,
			},
			agentName:  "agent-001",
			sessionID:  "session-001",
			sequence:   11,
			renewToken: "wrong-token",
			wantErr:    true,
		},
		{
			name: "repeated sequence is rejected",
			session: &model.AgentSession{
				AgentName:      "agent-001",
				SessionID:      "session-001",
				Status:         model.SessionStatusOnline,
				LastSequence:   10,
				LastRenewToken: "token-test",
				LastSeenAt:     now,
				StartedAt:      now,
			},
			agentName:  "agent-001",
			sessionID:  "session-001",
			sequence:   10,
			renewToken: "token-test",
			wantErr:    true,
		},
		{
			name: "skipped sequence is rejected",
			session: &model.AgentSession{
				AgentName:      "agent-001",
				SessionID:      "session-001",
				Status:         model.SessionStatusOnline,
				LastSequence:   10,
				LastRenewToken: "token-test",
				LastSeenAt:     now,
				StartedAt:      now,
			},
			agentName:  "agent-001",
			sessionID:  "session-001",
			sequence:   12,
			renewToken: "token-test",
			wantErr:    true,
		},
		{
			name: "wrong agent is rejected",
			session: &model.AgentSession{
				AgentName:      "agent-001",
				SessionID:      "session-001",
				Status:         model.SessionStatusOnline,
				LastSequence:   10,
				LastRenewToken: "token-test",
				LastSeenAt:     now,
				StartedAt:      now,
			},
			agentName:  "agent-002",
			sessionID:  "session-001",
			sequence:   11,
			renewToken: "token-test",
			wantErr:    true,
		},
		{
			name: "ended session is rejected",
			session: &model.AgentSession{
				AgentName:      "agent-001",
				SessionID:      "session-001",
				Status:         model.SessionStatusEnded,
				LastSequence:   10,
				LastRenewToken: "token-test",
				LastSeenAt:     now,
				StartedAt:      now,
				EndedAt:        now,
			},
			agentName:  "agent-001",
			sessionID:  "session-001",
			sequence:   11,
			renewToken: "token-test",
			wantErr:    true,
		},
		{
			name:       "missing session is rejected",
			session:    nil,
			agentName:  "agent-001",
			sessionID:  "missing-session",
			sequence:   1,
			renewToken: "token-test",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			sessionStore := NewSessionStore(db)

			if tt.session != nil {
				if err := db.Create(tt.session).Error; err != nil {
					t.Fatalf("create session failed: %v", err)
				}
			}

			err := sessionStore.VerifyHeartbeat(
				tt.agentName,
				tt.sessionID,
				tt.sequence,
				tt.renewToken,
			)

			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestSessionStoreMarkExpiredBefore(t *testing.T) {
	now := time.Now()
	oldLastSeenAt := now.Add(-time.Hour)
	freshLastSeenAt := now
	cutoff := now.Add(-30 * time.Second)

	tests := []struct {
		name          string
		lastSeenAt    time.Time
		initialStatus string
		wantStatus    string
		wantEndedAt   bool
	}{
		{
			name:          "old online session is ended",
			lastSeenAt:    oldLastSeenAt,
			initialStatus: model.SessionStatusOnline,
			wantStatus:    model.SessionStatusEnded,
			wantEndedAt:   true,
		},
		{
			name:          "fresh online session stays online",
			lastSeenAt:    freshLastSeenAt,
			initialStatus: model.SessionStatusOnline,
			wantStatus:    model.SessionStatusOnline,
			wantEndedAt:   false,
		},
		{
			name:          "old ended session stays ended",
			lastSeenAt:    oldLastSeenAt,
			initialStatus: model.SessionStatusEnded,
			wantStatus:    model.SessionStatusEnded,
			wantEndedAt:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			sessionStore := NewSessionStore(db)

			session := model.AgentSession{
				AgentName:      "agent-001",
				SessionID:      "session-001",
				Status:         tt.initialStatus,
				LastSequence:   0,
				LastRenewToken: "token-test",
				LastSeenAt:     tt.lastSeenAt,
				StartedAt:      tt.lastSeenAt,
			}

			if err := db.Create(&session).Error; err != nil {
				t.Fatalf("create session failed: %v", err)
			}

			if err := sessionStore.MarkExpiredBefore(cutoff); err != nil {
				t.Fatalf("mark expired before failed: %v", err)
			}

			var got model.AgentSession
			if err := db.Where("session_id = ?", session.SessionID).First(&got).Error; err != nil {
				t.Fatalf("query session failed: %v", err)
			}

			if got.Status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", got.Status, tt.wantStatus)
			}
			if tt.wantEndedAt && got.EndedAt.IsZero() {
				t.Fatal("ended_at is zero")
			}
			if !tt.wantEndedAt && !got.EndedAt.IsZero() {
				t.Fatalf("ended_at = %v, want zero", got.EndedAt)
			}
		})
	}
}

func TestSessionStoreCreateSessionInitializesFields(t *testing.T) {
	db := setupTestDB(t)
	sessionStore := NewSessionStore(db)

	err := sessionStore.CreateSession(
		"agent-001",
		"test-host",
		"v0.0.1",
		"session-001",
		"token-001",
	)
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	var got model.AgentSession
	if err := db.Where("session_id = ?", "session-001").First(&got).Error; err != nil {
		t.Fatalf("query session failed: %v", err)
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
	if got.Status != model.SessionStatusOnline {
		t.Fatalf("status = %q, want %q", got.Status, model.SessionStatusOnline)
	}
	if got.LastSequence != 0 {
		t.Fatalf("last_sequence = %d, want 0", got.LastSequence)
	}
	if got.LastRenewToken != "token-001" {
		t.Fatalf("last_renew_token = %q, want %q", got.LastRenewToken, "token-001")
	}
	if got.LastSeenAt.IsZero() {
		t.Fatal("last_seen_at is zero")
	}
	if got.StartedAt.IsZero() {
		t.Fatal("started_at is zero")
	}
	if !got.EndedAt.IsZero() {
		t.Fatalf("ended_at = %v, want zero", got.EndedAt)
	}
}

func TestSessionStoreUpdateHeartbeat(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		session *model.AgentSession
		wantErr bool
	}{
		{
			name: "online session is updated",
			session: &model.AgentSession{
				AgentName:      "agent-001",
				SessionID:      "session-001",
				Status:         model.SessionStatusOnline,
				LastSequence:   10,
				LastRenewToken: "token-old",
				LastSeenAt:     now.Add(-time.Minute),
				StartedAt:      now.Add(-time.Minute),
			},
			wantErr: false,
		},
		{
			name:    "missing session is rejected",
			session: nil,
			wantErr: true,
		},
		{
			name: "ended session is rejected",
			session: &model.AgentSession{
				AgentName:      "agent-001",
				SessionID:      "session-001",
				Status:         model.SessionStatusEnded,
				LastSequence:   10,
				LastRenewToken: "token-old",
				LastSeenAt:     now.Add(-time.Minute),
				StartedAt:      now.Add(-time.Minute),
				EndedAt:        now.Add(-time.Second),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			sessionStore := NewSessionStore(db)

			if tt.session != nil {
				if err := db.Create(tt.session).Error; err != nil {
					t.Fatalf("create session failed: %v", err)
				}
			}

			err := sessionStore.UpdateHeartbeat("agent-001", "session-001", 11, "token-new")

			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}

			if tt.wantErr || tt.session == nil {
				return
			}

			var got model.AgentSession
			if err := db.Where("session_id = ?", "session-001").First(&got).Error; err != nil {
				t.Fatalf("query session failed: %v", err)
			}

			if got.LastSequence != 11 {
				t.Fatalf("last_sequence = %d, want 11", got.LastSequence)
			}
			if got.LastRenewToken != "token-new" {
				t.Fatalf("last_renew_token = %q, want %q", got.LastRenewToken, "token-new")
			}
			if got.Status != model.SessionStatusOnline {
				t.Fatalf("status = %q, want %q", got.Status, model.SessionStatusOnline)
			}
			if !got.LastSeenAt.After(now.Add(-time.Minute)) {
				t.Fatalf("last_seen_at = %v, want after %v", got.LastSeenAt, now.Add(-time.Minute))
			}
		})
	}
}

func TestSessionStoreAcceptHeartbeat(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name              string
		status            string
		lastSequence      int64
		storedRenewToken  string
		sequence          int64
		currentRenewToken string
		wantRejected      bool
	}{
		{
			name:              "valid heartbeat is accepted",
			status:            model.SessionStatusOnline,
			lastSequence:      10,
			storedRenewToken:  "token-old",
			sequence:          11,
			currentRenewToken: "token-old",
		},
		{
			name:              "wrong token is rejected",
			status:            model.SessionStatusOnline,
			lastSequence:      10,
			storedRenewToken:  "token-old",
			sequence:          11,
			currentRenewToken: "token-wrong",
			wantRejected:      true,
		},
		{
			name:              "wrong sequence is rejected",
			status:            model.SessionStatusOnline,
			lastSequence:      10,
			storedRenewToken:  "token-old",
			sequence:          12,
			currentRenewToken: "token-old",
			wantRejected:      true,
		},
		{
			name:              "ended session is rejected",
			status:            model.SessionStatusEnded,
			lastSequence:      10,
			storedRenewToken:  "token-old",
			sequence:          11,
			currentRenewToken: "token-old",
			wantRejected:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupTestDB(t)
			sessionStore := NewSessionStore(db)
			lastSeenAt := now.Add(-time.Minute)
			session := model.AgentSession{
				AgentName:      "agent-001",
				SessionID:      "session-001",
				Status:         tt.status,
				LastSequence:   tt.lastSequence,
				LastRenewToken: tt.storedRenewToken,
				LastSeenAt:     lastSeenAt,
				StartedAt:      lastSeenAt,
			}
			if err := db.Create(&session).Error; err != nil {
				t.Fatalf("create session failed: %v", err)
			}

			err := sessionStore.AcceptHeartbeat(
				"agent-001",
				"session-001",
				tt.sequence,
				tt.currentRenewToken,
				"token-new",
			)
			if tt.wantRejected {
				if !errors.Is(err, heartbeat.ErrRejected) {
					t.Fatalf("error = %v, want heartbeat.ErrRejected", err)
				}
			} else if err != nil {
				t.Fatalf("AcceptHeartbeat() error = %v", err)
			}

			var got model.AgentSession
			if err := db.Where("session_id = ?", "session-001").First(&got).Error; err != nil {
				t.Fatalf("query session failed: %v", err)
			}

			if tt.wantRejected {
				if got.LastSequence != tt.lastSequence || got.LastRenewToken != tt.storedRenewToken || !got.LastSeenAt.Equal(lastSeenAt) {
					t.Fatalf("rejected heartbeat changed session: sequence=%d token=%q last_seen_at=%v", got.LastSequence, got.LastRenewToken, got.LastSeenAt)
				}
				return
			}

			if got.LastSequence != tt.sequence {
				t.Fatalf("last_sequence = %d, want %d", got.LastSequence, tt.sequence)
			}
			if got.LastRenewToken != "token-new" {
				t.Fatalf("last_renew_token = %q, want token-new", got.LastRenewToken)
			}
			if !got.LastSeenAt.After(lastSeenAt) {
				t.Fatalf("last_seen_at = %v, want after %v", got.LastSeenAt, lastSeenAt)
			}
		})
	}
}

func TestSessionStoreAcceptHeartbeatConcurrentDuplicate(t *testing.T) {
	db := setupTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db failed: %v", err)
	}
	// A single connection keeps SQLite's in-memory database shared by both calls
	// while still exercising the conditional update from concurrent goroutines.
	sqlDB.SetMaxOpenConns(1)

	lastSeenAt := time.Now().Add(-time.Minute)
	session := model.AgentSession{
		AgentName:      "agent-001",
		SessionID:      "session-001",
		Status:         model.SessionStatusOnline,
		LastSequence:   10,
		LastRenewToken: "token-old",
		LastSeenAt:     lastSeenAt,
		StartedAt:      lastSeenAt,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	sessionStore := NewSessionStore(db)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- sessionStore.AcceptHeartbeat(
				"agent-001",
				"session-001",
				11,
				"token-old",
				"token-new",
			)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	succeeded := 0
	rejected := 0
	for err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, heartbeat.ErrRejected):
			rejected++
		default:
			t.Fatalf("unexpected AcceptHeartbeat() error: %v", err)
		}
	}

	if succeeded != 1 || rejected != 1 {
		t.Fatalf("results: succeeded=%d rejected=%d, want 1 each", succeeded, rejected)
	}
}

func TestSessionStoreExpirationBeforeHeartbeatRejectsHeartbeat(t *testing.T) {
	db := setupTestDB(t)
	sessionStore := NewSessionStore(db)
	now := time.Now()
	session := model.AgentSession{
		AgentName:      "agent-001",
		SessionID:      "session-001",
		Status:         model.SessionStatusOnline,
		LastSequence:   10,
		LastRenewToken: "token-old",
		LastSeenAt:     now.Add(-time.Minute),
		StartedAt:      now.Add(-time.Minute),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	if err := sessionStore.MarkExpiredBefore(now.Add(-30 * time.Second)); err != nil {
		t.Fatalf("MarkExpiredBefore() error = %v", err)
	}
	if err := sessionStore.AcceptHeartbeat("agent-001", "session-001", 11, "token-old", "token-new"); !errors.Is(err, heartbeat.ErrRejected) {
		t.Fatalf("AcceptHeartbeat() error = %v, want heartbeat.ErrRejected", err)
	}

	var got model.AgentSession
	if err := db.Where("session_id = ?", session.SessionID).First(&got).Error; err != nil {
		t.Fatalf("query session failed: %v", err)
	}
	if got.Status != model.SessionStatusEnded {
		t.Fatalf("status = %q, want %q", got.Status, model.SessionStatusEnded)
	}
	if got.LastSequence != 10 || got.LastRenewToken != "token-old" {
		t.Fatalf("rejected heartbeat changed credentials: sequence=%d token=%q", got.LastSequence, got.LastRenewToken)
	}
}

func TestSessionStoreHeartbeatBeforeExpirationKeepsSessionOnline(t *testing.T) {
	db := setupTestDB(t)
	sessionStore := NewSessionStore(db)
	now := time.Now()
	cutoff := now.Add(-30 * time.Second)
	session := model.AgentSession{
		AgentName:      "agent-001",
		SessionID:      "session-001",
		Status:         model.SessionStatusOnline,
		LastSequence:   10,
		LastRenewToken: "token-old",
		LastSeenAt:     now.Add(-time.Minute),
		StartedAt:      now.Add(-time.Minute),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	if err := sessionStore.AcceptHeartbeat("agent-001", "session-001", 11, "token-old", "token-new"); err != nil {
		t.Fatalf("AcceptHeartbeat() error = %v", err)
	}
	if err := sessionStore.MarkExpiredBefore(cutoff); err != nil {
		t.Fatalf("MarkExpiredBefore() error = %v", err)
	}

	var got model.AgentSession
	if err := db.Where("session_id = ?", session.SessionID).First(&got).Error; err != nil {
		t.Fatalf("query session failed: %v", err)
	}
	if got.Status != model.SessionStatusOnline {
		t.Fatalf("status = %q, want %q", got.Status, model.SessionStatusOnline)
	}
	if got.LastSequence != 11 || got.LastRenewToken != "token-new" {
		t.Fatalf("accepted heartbeat not retained: sequence=%d token=%q", got.LastSequence, got.LastRenewToken)
	}
	if !got.EndedAt.IsZero() {
		t.Fatalf("ended_at = %v, want zero", got.EndedAt)
	}
}

func TestSessionStoreNewSessionStartsFromSequenceOne(t *testing.T) {
	db := setupTestDB(t)
	sessionStore := NewSessionStore(db)

	err := sessionStore.CreateSession(
		"agent-001",
		"test-host",
		"v0.0.1",
		"session-001",
		"token-001",
	)
	if err != nil {
		t.Fatalf("create first session failed: %v", err)
	}

	err = sessionStore.UpdateHeartbeat("agent-001", "session-001", 1, "token-002")
	if err != nil {
		t.Fatalf("update first session failed: %v", err)
	}

	err = sessionStore.CreateSession(
		"agent-001",
		"test-host",
		"v0.0.1",
		"session-002",
		"token-101",
	)
	if err != nil {
		t.Fatalf("create second session failed: %v", err)
	}

	err = sessionStore.VerifyHeartbeat("agent-001", "session-002", 1, "token-101")
	if err != nil {
		t.Fatalf("new session sequence 1 should pass: %v", err)
	}
}

func TestSessionStoreExpiringOldSessionDoesNotAffectNewSession(t *testing.T) {
	db := setupTestDB(t)
	sessionStore := NewSessionStore(db)

	now := time.Now()
	oldLastSeenAt := now.Add(-time.Hour)
	cutoff := now.Add(-30 * time.Second)

	oldSession := model.AgentSession{
		AgentName:      "agent-001",
		SessionID:      "session-old",
		Status:         model.SessionStatusOnline,
		LastSequence:   0,
		LastRenewToken: "old-token",
		LastSeenAt:     oldLastSeenAt,
		StartedAt:      oldLastSeenAt,
	}
	newSession := model.AgentSession{
		AgentName:      "agent-001",
		SessionID:      "session-new",
		Status:         model.SessionStatusOnline,
		LastSequence:   0,
		LastRenewToken: "new-token",
		LastSeenAt:     now,
		StartedAt:      now,
	}

	if err := db.Create(&oldSession).Error; err != nil {
		t.Fatalf("create old session failed: %v", err)
	}
	if err := db.Create(&newSession).Error; err != nil {
		t.Fatalf("create new session failed: %v", err)
	}

	if err := sessionStore.MarkExpiredBefore(cutoff); err != nil {
		t.Fatalf("mark expired before failed: %v", err)
	}

	var gotOld model.AgentSession
	if err := db.Where("session_id = ?", oldSession.SessionID).First(&gotOld).Error; err != nil {
		t.Fatalf("query old session failed: %v", err)
	}
	if gotOld.Status != model.SessionStatusEnded {
		t.Fatalf("old session status = %q, want %q", gotOld.Status, model.SessionStatusEnded)
	}

	var gotNew model.AgentSession
	if err := db.Where("session_id = ?", newSession.SessionID).First(&gotNew).Error; err != nil {
		t.Fatalf("query new session failed: %v", err)
	}
	if gotNew.Status != model.SessionStatusOnline {
		t.Fatalf("new session status = %q, want %q", gotNew.Status, model.SessionStatusOnline)
	}

	err := sessionStore.VerifyHeartbeat("agent-001", newSession.SessionID, 1, "new-token")
	if err != nil {
		t.Fatalf("new session heartbeat should pass: %v", err)
	}
}
