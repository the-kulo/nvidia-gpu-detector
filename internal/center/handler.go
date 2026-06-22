package center

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/heartbeat"
	"github.com/the-kulo/nvidia-gpu-detector/internal/token"
)

func (s *Server) HeartbeatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req heartbeat.HeartbeatRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "bad Request", http.StatusBadRequest)
		return
	}

	if req.AgentName == "" || req.SessionID == "" || req.Sequence <= 0 {
		http.Error(w, "bad Request", http.StatusBadRequest)
		return
	}

	err = s.sessionStore.VerifyHeartbeat(
		req.AgentName,
		req.SessionID,
		req.Sequence,
		req.RenewToken,
	)
	if err != nil {
		http.Error(w, "heartbeat verify failed", http.StatusUnauthorized)
		return
	}

	newRenewtoken, err := token.GenerateRenewToken()
	if err != nil {
		http.Error(w, "generate token failed", http.StatusInternalServerError)
		return
	}

	resp := heartbeat.HeartbeatResponse{
		ServerTime:       time.Now(),
		RenewToken:       newRenewtoken,
		NextHeartbeatSec: 10,
	}

	err = s.sessionStore.UpdateHeartbeat(
		req.AgentName,
		req.SessionID,
		req.Sequence,
		resp.RenewToken,
	)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	err = s.agentStore.UpdateLastSeen(
		req.AgentName,
		req.Hostname,
		req.Version,
	)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(resp)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

}
