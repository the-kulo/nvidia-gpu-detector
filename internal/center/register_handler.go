package center

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/session"
	"github.com/the-kulo/nvidia-gpu-detector/internal/token"
)

func (s *Server) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req session.RegisterRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if req.AgentName == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	err = s.agentStore.UpdateLastSeen(req.AgentName, req.Hostname, req.Version)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	sessionID, err := token.GenerateSessionID()
	if err != nil {
		http.Error(w, "generate session failed", http.StatusInternalServerError)
		return
	}

	renewToken, err := token.GenerateRenewToken()
	if err != nil {
		http.Error(w, "generate token failed", http.StatusInternalServerError)
		return
	}

	err = s.sessionStore.CreateSession(
		req.AgentName,
		req.Hostname,
		req.Version,
		sessionID,
		renewToken,
	)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	resp := session.RegisterResponse{
		SessionID:        sessionID,
		RenewToken:       renewToken,
		ServerTime:       time.Now(),
		NextHeartbeatSec: 10,
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(resp)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}
