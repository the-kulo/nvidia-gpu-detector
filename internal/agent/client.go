package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/heartbeat"
	"github.com/the-kulo/nvidia-gpu-detector/internal/session"
)

type Config struct {
	AgentName string
	Hostname  string
	Version   string
	CenterURL string
}

func StartAgent(cfg Config) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	var sequence int64 = 0
	nextHeartbeatSec := 10
	sessionID := ""
	renewToken := ""

	for {
		registerResp, err := registerSession(client, cfg.CenterURL, session.RegisterRequest{
			AgentName: cfg.AgentName,
			Hostname:  cfg.Hostname,
			Version:   cfg.Version,
		})
		if err != nil {
			fmt.Println("register session failed:", err)
			time.Sleep(time.Second * 10)
			continue
		}

		sessionID = registerResp.SessionID
		renewToken = registerResp.RenewToken

		if registerResp.NextHeartbeatSec > 0 {
			nextHeartbeatSec = registerResp.NextHeartbeatSec
		}

		fmt.Println("register session ok")
		fmt.Println("session_id:", sessionID)
		fmt.Println("server_time:", registerResp.ServerTime)
		break
	}

	for {
		sequence++

		reqBody := heartbeat.HeartbeatRequest{
			AgentName:  cfg.AgentName,
			SessionID:  sessionID,
			Hostname:   cfg.Hostname,
			Timestamp:  time.Now(),
			Sequence:   sequence,
			Version:    cfg.Version,
			RenewToken: renewToken,
		}

		resp, err := sendHeartbeat(client, cfg.CenterURL, reqBody)
		if err != nil {
			fmt.Println("send heartbeat failed:", err)

			time.Sleep(time.Second * 10)
			continue
		}

		renewToken = resp.RenewToken

		if resp.NextHeartbeatSec > 0 {
			nextHeartbeatSec = resp.NextHeartbeatSec
		}

		fmt.Println("heartbeat ok")
		fmt.Println("server_time:", resp.ServerTime)
		fmt.Println("renew_token:", renewToken)
		fmt.Println("next_heartbeat_sec:", nextHeartbeatSec)

		time.Sleep(time.Duration(nextHeartbeatSec) * time.Second)
	}
}

func registerSession(client *http.Client, centerURL string, reqBody session.RegisterRequest) (session.RegisterResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return session.RegisterResponse{}, err
	}

	req, err := http.NewRequest(http.MethodPost, centerEndpoint(centerURL, "/agent/register"), bytes.NewReader(body))
	if err != nil {
		return session.RegisterResponse{}, err
	}

	req.Header.Set("Content-type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return session.RegisterResponse{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return session.RegisterResponse{}, fmt.Errorf("register failed, status: %d", resp.StatusCode)
	}

	var registerResp session.RegisterResponse
	err = json.NewDecoder(resp.Body).Decode(&registerResp)
	if err != nil {
		return session.RegisterResponse{}, err
	}

	return registerResp, nil
}

func sendHeartbeat(client *http.Client, centerURL string, reqBody heartbeat.HeartbeatRequest) (heartbeat.HeartbeatResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return heartbeat.HeartbeatResponse{}, err
	}

	req, err := http.NewRequest(http.MethodPost, centerEndpoint(centerURL, "/heartbeat"), bytes.NewReader(body))
	if err != nil {
		return heartbeat.HeartbeatResponse{}, err
	}

	req.Header.Set("Content-type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return heartbeat.HeartbeatResponse{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return heartbeat.HeartbeatResponse{}, fmt.Errorf("heartbeat failed, status: %d", resp.StatusCode)
	}

	var heartbeatResp heartbeat.HeartbeatResponse
	err = json.NewDecoder(resp.Body).Decode(&heartbeatResp)
	if err != nil {
		return heartbeat.HeartbeatResponse{}, err
	}

	return heartbeatResp, nil
}

func centerEndpoint(centerURL string, path string) string {
	return strings.TrimRight(centerURL, "/") + path
}
