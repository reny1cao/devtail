package tailscale

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

type Client struct {
	apiKey  string
	tailnet string
	baseURL string
	http    *http.Client
}

type AuthKey struct {
	ID           string    `json:"id"`
	Key          string    `json:"key"`
	Created      time.Time `json:"created"`
	Expires      time.Time `json:"expires"`
	Capabilities Capabilities `json:"capabilities"`
}

type Capabilities struct {
	Devices struct {
		Create struct {
			Reusable      bool     `json:"reusable"`
			Ephemeral     bool     `json:"ephemeral"`
			Tags          []string `json:"tags"`
			PreAuthorized bool     `json:"preauthorized"`
		} `json:"create"`
	} `json:"devices"`
}

func NewClient(apiKey, tailnet string) *Client {
	return &Client{
		apiKey:  apiKey,
		tailnet: tailnet,
		baseURL: "https://api.tailscale.com/api/v2",
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) CreateAuthKey(ctx context.Context, description string) (*AuthKey, error) {
	payload := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"devices": map[string]interface{}{
				"create": map[string]interface{}{
					"reusable":      false,
					"ephemeral":     false,
					"tags":          []string{"tag:devtail"},
					"preauthorized": true,
				},
			},
		},
		"expirySeconds": 3600, // 1 hour
		"description":   description,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/tailnet/%s/keys", c.baseURL, c.tailnet)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tailscale API error: %s - %s", resp.Status, string(body))
	}

	var authKey AuthKey
	if err := json.Unmarshal(body, &authKey); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	log.Info().
		Str("key_id", authKey.ID).
		Str("description", description).
		Msg("Tailscale auth key created")

	return &authKey, nil
}

func (c *Client) DeleteAuthKey(ctx context.Context, keyID string) error {
	url := fmt.Sprintf("%s/tailnet/%s/keys/%s", c.baseURL, c.tailnet, keyID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("tailscale API error: %s - %s", resp.Status, string(body))
	}

	log.Info().
		Str("key_id", keyID).
		Msg("Tailscale auth key deleted")

	return nil
}

type Device struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Hostname   string   `json:"hostname"`
	Addresses  []string `json:"addresses"`
	Tags       []string `json:"tags"`
	LastSeen   string   `json:"lastSeen"`
	Online     bool     `json:"online"`
}

func (c *Client) GetDeviceByHostname(ctx context.Context, hostname string) (*Device, error) {
	url := fmt.Sprintf("%s/tailnet/%s/devices", c.baseURL, c.tailnet)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tailscale API error: %s - %s", resp.Status, string(body))
	}

	var response struct {
		Devices []Device `json:"devices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	for _, device := range response.Devices {
		if device.Hostname == hostname {
			return &device, nil
		}
	}

	return nil, fmt.Errorf("device not found: %s", hostname)
}

func (c *Client) WaitForDevice(ctx context.Context, hostname string, timeout time.Duration) (*Device, error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	for {
		select {
		case <-ticker.C:
			device, err := c.GetDeviceByHostname(ctx, hostname)
			if err == nil && device.Online {
				return device, nil
			}
			// Continue polling if not found or not online
			
		case <-timeoutTimer.C:
			return nil, fmt.Errorf("timeout waiting for device %s", hostname)
			
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}