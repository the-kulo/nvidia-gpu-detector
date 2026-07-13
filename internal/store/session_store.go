package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/heartbeat"
	"github.com/the-kulo/nvidia-gpu-detector/internal/model"
	"gorm.io/gorm"
)

type SessionStore struct {
	db *gorm.DB
}

func NewSessionStore(db *gorm.DB) *SessionStore {
	return &SessionStore{
		db: db,
	}
}

func (s *SessionStore) CreateSession(
	agentName string,
	hostname string,
	version string,
	sessionID string,
	renewToken string,
) error {
	now := time.Now()

	session := model.AgentSession{
		AgentName:      agentName,
		SessionID:      sessionID,
		Hostname:       hostname,
		Version:        version,
		Status:         model.SessionStatusOnline,
		LastSequence:   0,
		LastRenewToken: renewToken,
		LastSeenAt:     now,
		StartedAt:      now,
	}

	err := s.db.Create(&session).Error
	if err != nil {
		return fmt.Errorf("create agent session failed: %w", err)
	}

	return nil
}

func (s *SessionStore) VerifyHeartbeat(
	agentName string,
	sessionID string,
	sequence int64,
	renewToken string,
) error {
	var session model.AgentSession

	err := s.db.
		Where("session_id = ?", sessionID).
		First(&session).
		Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("session not found")
		}

		return fmt.Errorf("query session failed: %w", err)
	}

	if session.AgentName != agentName {
		return fmt.Errorf("session does not belong to agent")
	}

	if session.Status != model.SessionStatusOnline {
		return fmt.Errorf("session is not online")
	}

	expectSequence := session.LastSequence + 1

	if sequence != expectSequence {
		return fmt.Errorf("invalid sequence: got %d, want %d", sequence, expectSequence)
	}

	if renewToken != session.LastRenewToken {
		return fmt.Errorf("invalid renew token")
	}

	return nil
}

func (s *SessionStore) UpdateHeartbeat(
	agentName string,
	sessionID string,
	sequence int64,
	newRenewToken string,
) error {
	now := time.Now()

	result := s.db.
		Model(&model.AgentSession{}).
		Where(
			"agent_name = ? AND session_id = ? AND status = ?",
			agentName,
			sessionID,
			model.SessionStatusOnline,
		).
		Updates(map[string]any{
			"last_sequence":    sequence,
			"last_renew_token": newRenewToken,
			"last_seen_at":     now,
			"status":           model.SessionStatusOnline,
		})

	if result.Error != nil {
		return fmt.Errorf("update session heartbeat failed: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("session not found or not online")
	}

	return nil
}

func (s *SessionStore) AcceptHeartbeat(
	agentName string,
	sessionID string,
	sequence int64,
	currentRenewToken string,
	newRenewToken string,
) error {
	result := s.db.
		Model(&model.AgentSession{}).
		Where(
			"agent_name = ? AND session_id = ? AND status = ? AND last_sequence = ? AND last_renew_token = ?",
			agentName,
			sessionID,
			model.SessionStatusOnline,
			sequence-1,
			currentRenewToken,
		).
		Updates(map[string]any{
			"last_sequence":    sequence,
			"last_renew_token": newRenewToken,
			"last_seen_at":     time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("accept session heartbeat failed: %w", result.Error)
	}

	if result.RowsAffected != 1 {
		return heartbeat.ErrRejected
	}

	return nil
}

func (s *SessionStore) MarkExpiredBefore(cutoff time.Time) error {
	now := time.Now()

	err := s.db.
		Model(&model.AgentSession{}).
		Where("status = ? AND last_seen_at < ?", model.SessionStatusOnline, cutoff).
		Updates(map[string]any{
			"status":   model.SessionStatusEnded,
			"ended_at": now,
		}).
		Error
	if err != nil {
		return fmt.Errorf("marked expired sessions failed: %w", err)
	}

	return nil
}
