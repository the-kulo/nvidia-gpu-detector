package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

type HTTPStatusError struct {
	Operation  string
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("%s failed, status: %d", e.Operation, e.StatusCode)
}

const (
	defaultHeartbeatInterval = 10 * time.Second
	defaultRetryInterval     = 10 * time.Second
)

func StartAgent(cfg Config) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	if err := runAgent(context.Background(), client, cfg, defaultRetryInterval); err != nil {
		fmt.Println("agent stopped:", err)
	}
}

func runAgent(ctx context.Context, client *http.Client, cfg Config, retryInterval time.Duration) error {
	for {
		registerResp, err := registerSession(ctx, client, cfg.CenterURL, session.RegisterRequest{
			AgentName: cfg.AgentName,
			Hostname:  cfg.Hostname,
			Version:   cfg.Version,
		})
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			fmt.Println("register session failed:", err)
			if err := waitFor(ctx, retryInterval); err != nil {
				return err
			}
			continue
		}

		sessionID := registerResp.SessionID
		renewToken := registerResp.RenewToken
		var sequence int64
		nextHeartbeatInterval := defaultHeartbeatInterval

		if registerResp.NextHeartbeatSec > 0 {
			nextHeartbeatInterval = time.Duration(registerResp.NextHeartbeatSec) * time.Second
		}

		fmt.Println("register session ok")
		fmt.Println("session_id:", sessionID)
		fmt.Println("server_time:", registerResp.ServerTime)

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

			resp, err := sendHeartbeat(ctx, client, cfg.CenterURL, reqBody)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}

				var statusErr *HTTPStatusError
				if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusUnauthorized {
					sessionID = ""
					renewToken = ""
					sequence = 0
					break
				}

				fmt.Println("send heartbeat failed:", err)
				if err := waitFor(ctx, retryInterval); err != nil {
					return err
				}
				continue
			}

			renewToken = resp.RenewToken

			if resp.NextHeartbeatSec > 0 {
				nextHeartbeatInterval = time.Duration(resp.NextHeartbeatSec) * time.Second
			}

			fmt.Println("heartbeat ok")
			fmt.Println("server_time:", resp.ServerTime)
			fmt.Println("renew_token:", renewToken)
			fmt.Println("next_heartbeat_sec:", int(nextHeartbeatInterval/time.Second))

			if err := waitFor(ctx, nextHeartbeatInterval); err != nil {
				return err
			}
		}
	}
}

func waitFor(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func registerSession(ctx context.Context, client *http.Client, centerURL string, reqBody session.RegisterRequest) (session.RegisterResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return session.RegisterResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, centerEndpoint(centerURL, "/agent/register"), bytes.NewReader(body))
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

func sendHeartbeat(ctx context.Context, client *http.Client, centerURL string, reqBody heartbeat.HeartbeatRequest) (heartbeat.HeartbeatResponse, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return heartbeat.HeartbeatResponse{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, centerEndpoint(centerURL, "/heartbeat"), bytes.NewReader(body))
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
		return heartbeat.HeartbeatResponse{}, &HTTPStatusError{
			Operation:  "heartbeat",
			StatusCode: resp.StatusCode,
		}
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
