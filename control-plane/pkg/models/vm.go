package models

import (
	"time"
)

type VMStatus string

const (
	VMStatusProvisioning VMStatus = "provisioning"
	VMStatusRunning      VMStatus = "running"
	VMStatusSuspended    VMStatus = "suspended"
	VMStatusError        VMStatus = "error"
	VMStatusTerminated   VMStatus = "terminated"
)

type VMSpec struct {
	Type     string `json:"type"`     // e.g., "cx11", "cx21"
	Location string `json:"location"` // e.g., "nbg1", "fsn1"
	DiskSize int    `json:"disk_size"` // in GB
}

type VM struct {
	ID               string    `json:"id" db:"id"`
	UserID           string    `json:"user_id" db:"user_id"`
	HetznerID        int64     `json:"hetzner_id" db:"hetzner_id"`
	TailscaleIP      string    `json:"tailscale_ip" db:"tailscale_ip"`
	TailscaleAuthKey string    `json:"-" db:"tailscale_auth_key"`
	Status           VMStatus  `json:"status" db:"status"`
	Spec             VMSpec    `json:"spec" db:"spec"`
	WebsocketToken   string    `json:"websocket_token" db:"websocket_token"`
	LastActivity     time.Time `json:"last_activity" db:"last_activity"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

type CreateVMRequest struct {
	UserID string `json:"user_id" binding:"required"`
	Spec   VMSpec `json:"spec" binding:"required"`
}

type CreateVMResponse struct {
	VM             *VM    `json:"vm"`
	WebsocketURL   string `json:"websocket_url"`
	EstimatedReady int    `json:"estimated_ready_seconds"`
}