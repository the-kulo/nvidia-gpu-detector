package center

import (
	"net/http"

	"github.com/the-kulo/nvidia-gpu-detector/internal/store"
)

type Server struct {
	agentStore *store.AgentStore
}

func NewServer(agentStore *store.AgentStore) *Server {
	return &Server{
		agentStore: agentStore,
	}
}

func (s *Server) StartServer(addr string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/heartbeat", s.HeartbeatHandler)

	return http.ListenAndServe(addr, mux)
}
