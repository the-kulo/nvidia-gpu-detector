package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/the-kulo/nvidia-gpu-detector/internal/heartbeat"
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
	renewToken := ""
	nextHeartbeatSec := 10

	for {
		sequence++

		reqBody := heartbeat.HeartbeatRequest{
			AgentName:  cfg.AgentName,
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

func sendHeartbeat(client *http.Client, url string, reqBody heartbeat.HeartbeatRequest) (heartbeat.HeartbeatResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return heartbeat.HeartbeatResponse{}, err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
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
