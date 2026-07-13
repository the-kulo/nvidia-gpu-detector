package center

import (
	"fmt"
	"net/http"
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/service"
	"github.com/the-kulo/nvidia-gpu-detector/internal/store"
)

type Server struct {
	agentStore       *store.AgentStore
	sessionStore     *store.SessionStore
	heartbeatService service.HeartbeatService
}

func NewServer(
	agentStore *store.AgentStore,
	sessionStore *store.SessionStore,
	heartbeatService service.HeartbeatService,
) *Server {
	return &Server{
		agentStore:       agentStore,
		sessionStore:     sessionStore,
		heartbeatService: heartbeatService,
	}
}

func (s *Server) StartServer(addr string) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/agent/register", s.RegisterHandler)
	mux.HandleFunc("/heartbeat", s.HeartbeatHandler)

	s.StatusMonitor(10*time.Second, 30*time.Second)

	return http.ListenAndServe(addr, mux)
}

func (s *Server) StatusMonitor(interval time.Duration, timeout time.Duration) {
	ticker := time.NewTicker(interval)

	go func() {
		for range ticker.C {
			cutoff := time.Now().Add(-timeout)

			if err := s.sessionStore.MarkExpiredBefore(cutoff); err != nil {
				fmt.Println("status monitor failed:", err)
			}

			if err := s.agentStore.MarkOfflineBefore(cutoff); err != nil {
				fmt.Println("status monitor failed:", err)
			}
		}
	}()
}
