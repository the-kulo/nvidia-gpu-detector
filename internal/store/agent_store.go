package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/model"
	"gorm.io/gorm"
)

type AgentStore struct {
	db *gorm.DB
}

func NewAgentStore(db *gorm.DB) *AgentStore {
	return &AgentStore{
		db: db,
	}
}

func (s *AgentStore) UpdateHeartbeat(
	agentName string,
	hostname string,
	version string,
	sequence int64,
	newRenewToken string,
) error {
	now := time.Now()

	var agent model.Agent

	err := s.db.Where("agent_name = ?", agentName).First(&agent).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			newAgent := model.Agent{
				AgentName:      agentName,
				Hostname:       hostname,
				Version:        version,
				Status:         model.AgentStatusOnline,
				LastSeenAt:     now,
				LastSequence:   sequence,
				LastRenewToken: newRenewToken,
			}

			if err := s.db.Create(&newAgent).Error; err != nil {
				return fmt.Errorf("create agent failed: %w", err)
			}

			return nil
		}
		return fmt.Errorf("query agent failed: %w", err)
	}

	err = s.db.Model(&agent).Updates(map[string]interface{}{
		"hostname":         hostname,
		"version":          version,
		"status":           model.AgentStatusOnline,
		"last_seen_at":     now,
		"last_sequence":    sequence,
		"last_renew_token": newRenewToken,
	}).Error

	if err != nil {
		return fmt.Errorf("update agent heartbeat failed: %w", err)
	}

	return nil

}
