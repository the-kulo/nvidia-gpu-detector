package session

import "time"

type RegisterRequest struct {
	AgentName string `json:"agent_name"`
	Hostname  string `json:"hostname"`
	Version   string `json:"version"`
}

type RegisterResponse struct {
	SessionID        string    `json:"session_id"`
	RenewToken       string    `json:"renew_token"`
	ServerTime       time.Time `json:"server_time"`
	NextHeartbeatSec int       `json:"next_heartbeat_sec"`
}
