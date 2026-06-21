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

func (s *AgentStore) UpdateLastSeen(
	agentName string,
	hostname string,
	version string,
) error {
	now := time.Now()

	agent, err := s.findAgentByName(agentName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.createAgent(agentName, hostname, version, now)
		}
		return fmt.Errorf("query agent failed: %w", err)
	}

	err = s.updateAgentLastSeen(&agent, hostname, version, now)
	if err != nil {
		return fmt.Errorf("update agent last seen failed: %w", err)
	}

	return nil
}

func (s *AgentStore) findAgentByName(agentName string) (model.Agent, error) {
	var agent model.Agent

	err := s.db.
		Where("agent_name = ?", agentName).
		First(&agent).
		Error
	if err != nil {
		return model.Agent{}, err
	}

	return agent, nil
}

func (s *AgentStore) createAgent(
	agentName string,
	hostname string,
	version string,
	now time.Time,
) error {
	agent := model.Agent{
		AgentName:  agentName,
		Hostname:   hostname,
		Version:    version,
		Status:     model.AgentStatusOnline,
		LastSeenAt: now,
	}

	err := s.db.Create(&agent).Error
	if err != nil {
		return fmt.Errorf("create agent failed: %w", err)
	}

	return nil
}

func (s *AgentStore) updateAgentLastSeen(
	agent *model.Agent,
	hostname string,
	version string,
	now time.Time,
) error {
	return s.db.Model(agent).Updates(map[string]interface{}{
		"hostname":     hostname,
		"version":      version,
		"status":       model.AgentStatusOnline,
		"last_seen_at": now,
	}).Error
}

func (s *AgentStore) MarkOfflineBefore(cutoff time.Time) error {
	err := s.db.
		Model(&model.Agent{}).
		Where("status = ? AND last_seen_at < ?", model.AgentStatusOnline, cutoff).
		Update("status", model.AgentStatusOffline).
		Error

	if err != nil {
		return fmt.Errorf("mark offline failed: %w", err)
	}

	return nil
}
