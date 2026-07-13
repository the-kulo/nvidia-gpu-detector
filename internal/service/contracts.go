package service

import (
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/heartbeat"
)

var ErrHeartbeatRejected = heartbeat.ErrRejected

type HeartbeatCommand struct {
	AgentName  string
	SessionID  string
	Hostname   string
	Version    string
	Sequence   int64
	RenewToken string
}

type HeartbeatResult struct {
	ServerTime       time.Time
	RenewToken       string
	NextHeartbeatSec int
}

type SessionStore interface {
	AcceptHeartbeat(
		agentName string,
		sessionID string,
		sequence int64,
		currentRenewToken string,
		newRenewToken string,
	) error
}

type AgentStore interface {
	UpdateLastSeen(agentName string, hostname string, version string) error
}

type HeartbeatService interface {
	HandleHeartbeat(command HeartbeatCommand) (HeartbeatResult, error)
}
