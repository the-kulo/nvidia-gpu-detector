package main

import (
	"os"

	"github.com/the-kulo/nvidia-gpu-detector/internal/agent"
)

func main() {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unkown hostname"
	}

	cfg := agent.Config{
		AgentName: "agent-001",
		Hostname:  hostname,
		Version:   "v0.0.1",
		CenterURL: "http://127.0.0.1:8080",
	}

	agent.StartAgent(cfg)
}
