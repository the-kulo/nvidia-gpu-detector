package center

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/heartbeat"
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

	if req.AgentName == "" || req.Sequence <= 0 {
		http.Error(w, "bad Request", http.StatusBadRequest)
		return
	}

	fmt.Println("receive heartbeat:")
	fmt.Println("agent_name:", req.AgentName)
	fmt.Println("hostname:", req.Hostname)
	fmt.Println("sequence:", req.Sequence)
	fmt.Println("version:", req.Version)
	fmt.Println("renew_token:", req.RenewToken)

	resp := heartbeat.HeartbeatResponse{
		ServerTime:       time.Now(),
		RenewToken:       "test-token",
		NextHeartbeatSec: 10,
	}

	err = s.agentStore.UpdateHeartbeat(
		req.AgentName,
		req.Hostname,
		req.Version,
		req.Sequence,
		resp.RenewToken,
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
