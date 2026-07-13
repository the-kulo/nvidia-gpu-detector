package service

import (
	"fmt"
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/token"
)

const nextHeartbeatSeconds = 10

type heartbeatService struct {
	agentStore    AgentStore
	sessionStore  SessionStore
	generateToken func() (string, error)
	now           func() time.Time
}

func NewHeartbeatService(agentStore AgentStore, sessionStore SessionStore) HeartbeatService {
	return newHeartbeatService(agentStore, sessionStore, token.GenerateRenewToken, time.Now)
}

func newHeartbeatService(
	agentStore AgentStore,
	sessionStore SessionStore,
	generateToken func() (string, error),
	now func() time.Time,
) HeartbeatService {
	return &heartbeatService{
		agentStore:    agentStore,
		sessionStore:  sessionStore,
		generateToken: generateToken,
		now:           now,
	}
}

func (s *heartbeatService) HandleHeartbeat(command HeartbeatCommand) (HeartbeatResult, error) {
	newRenewToken, err := s.generateToken()
	if err != nil {
		return HeartbeatResult{}, fmt.Errorf("generate heartbeat renew token: %w", err)
	}

	err = s.sessionStore.AcceptHeartbeat(
		command.AgentName,
		command.SessionID,
		command.Sequence,
		command.RenewToken,
		newRenewToken,
	)
	if err != nil {
		return HeartbeatResult{}, fmt.Errorf("accept heartbeat: %w", err)
	}

	if err := s.agentStore.UpdateLastSeen(command.AgentName, command.Hostname, command.Version); err != nil {
		return HeartbeatResult{}, fmt.Errorf("update agent last seen: %w", err)
	}

	return HeartbeatResult{
		ServerTime:       s.now(),
		RenewToken:       newRenewToken,
		NextHeartbeatSec: nextHeartbeatSeconds,
	}, nil
}
