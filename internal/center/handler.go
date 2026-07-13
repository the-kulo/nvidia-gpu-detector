package center

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/the-kulo/nvidia-gpu-detector/internal/heartbeat"
	"github.com/the-kulo/nvidia-gpu-detector/internal/service"
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

	result, err := s.heartbeatService.HandleHeartbeat(service.HeartbeatCommand{
		AgentName:  req.AgentName,
		SessionID:  req.SessionID,
		Hostname:   req.Hostname,
		Version:    req.Version,
		Sequence:   req.Sequence,
		RenewToken: req.RenewToken,
	})
	if err != nil {
		if errors.Is(err, service.ErrHeartbeatRejected) {
			http.Error(w, "heartbeat rejected", http.StatusUnauthorized)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	resp := heartbeat.HeartbeatResponse{
		ServerTime:       result.ServerTime,
		RenewToken:       result.RenewToken,
		NextHeartbeatSec: result.NextHeartbeatSec,
	}

	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(resp)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

}
