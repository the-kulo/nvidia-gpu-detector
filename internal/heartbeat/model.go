package heartbeat

import (
	"time"
)

type HeartbeatRequest struct {
	AgentName  string    `json:"agent_name"`
	Hostname   string    `json:"hostname"`
	Timestamp  time.Time `json:"timestamp"`
	Sequence   int64     `json:"sequence"`
	Version    string    `json:"version"`
	RenewToken string    `json:"renew_token"`
}

type HeartbeatResponse struct {
	ServerTime       time.Time `json:"server_time"`
	RenewToken       string    `json:"renew_token"`
	NextHeartbeatSec int       `json:"next_heartbeat_sec"`
}
