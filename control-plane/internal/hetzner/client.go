package hetzner

import (
	"context"
	"fmt"
	"time"

	"github.com/devtail/control-plane/pkg/models"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/rs/zerolog/log"
)

type Client struct {
	client    *hcloud.Client
	sshKeyID  int64
	networkID int64
}

func NewClient(token string, sshKeyID, networkID int64) *Client {
	return &Client{
		client:    hcloud.NewClient(hcloud.WithToken(token)),
		sshKeyID:  sshKeyID,
		networkID: networkID,
	}
}

func (c *Client) CreateVM(ctx context.Context, vm *models.VM, cloudInitScript string) error {
	serverType, err := c.client.ServerType.GetByName(ctx, vm.Spec.Type)
	if err != nil {
		return fmt.Errorf("get server type: %w", err)
	}

	location, err := c.client.Location.GetByName(ctx, vm.Spec.Location)
	if err != nil {
		return fmt.Errorf("get location: %w", err)
	}

	image, err := c.client.Image.GetByName(ctx, "ubuntu-22.04")
	if err != nil {
		return fmt.Errorf("get image: %w", err)
	}

	sshKey, err := c.client.SSHKey.GetByID(ctx, c.sshKeyID)
	if err != nil {
		return fmt.Errorf("get ssh key: %w", err)
	}

	network, err := c.client.Network.GetByID(ctx, c.networkID)
	if err != nil && c.networkID != 0 {
		return fmt.Errorf("get network: %w", err)
	}

	opts := hcloud.ServerCreateOpts{
		Name:       fmt.Sprintf("devtail-%s", vm.ID),
		ServerType: serverType,
		Image:      image,
		Location:   location,
		SSHKeys:    []*hcloud.SSHKey{sshKey},
		UserData:   cloudInitScript,
		Labels: map[string]string{
			"user_id":    vm.UserID,
			"vm_id":      vm.ID,
			"created_at": vm.CreatedAt.Format(time.RFC3339),
		},
	}

	if network != nil {
		opts.Networks = []*hcloud.Network{network}
	}

	result, _, err := c.client.Server.Create(ctx, opts)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}

	vm.HetznerID = result.Server.ID
	
	log.Info().
		Int64("hetzner_id", result.Server.ID).
		Str("vm_id", vm.ID).
		Msg("VM created in Hetzner")

	// Wait for the server to get an IP
	server, err := c.waitForIP(ctx, result.Server.ID)
	if err != nil {
		return fmt.Errorf("wait for IP: %w", err)
	}

	log.Info().
		Str("public_ip", server.PublicNet.IPv4.IP.String()).
		Str("vm_id", vm.ID).
		Msg("VM received public IP")

	return nil
}

func (c *Client) waitForIP(ctx context.Context, serverID int64) (*hcloud.Server, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeout := time.NewTimer(60 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-ticker.C:
			server, _, err := c.client.Server.GetByID(ctx, serverID)
			if err != nil {
				return nil, err
			}
			
			if server.PublicNet.IPv4 != nil && server.PublicNet.IPv4.IP != nil {
				return server, nil
			}
			
		case <-timeout.C:
			return nil, fmt.Errorf("timeout waiting for server IP")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (c *Client) DeleteVM(ctx context.Context, hetznerID int64) error {
	server, _, err := c.client.Server.GetByID(ctx, hetznerID)
	if err != nil {
		return fmt.Errorf("get server: %w", err)
	}

	if server == nil {
		return nil // Already deleted
	}

	_, _, err = c.client.Server.Delete(ctx, server)
	if err != nil {
		return fmt.Errorf("delete server: %w", err)
	}

	log.Info().
		Int64("hetzner_id", hetznerID).
		Msg("VM deleted from Hetzner")

	return nil
}

func (c *Client) GetVM(ctx context.Context, hetznerID int64) (*hcloud.Server, error) {
	server, _, err := c.client.Server.GetByID(ctx, hetznerID)
	if err != nil {
		return nil, fmt.Errorf("get server: %w", err)
	}
	return server, nil
}

func (c *Client) PowerOffVM(ctx context.Context, hetznerID int64) error {
	server, _, err := c.client.Server.GetByID(ctx, hetznerID)
	if err != nil {
		return fmt.Errorf("get server: %w", err)
	}

	action, _, err := c.client.Server.Poweroff(ctx, server)
	if err != nil {
		return fmt.Errorf("poweroff server: %w", err)
	}

	if err := c.waitForAction(ctx, action); err != nil {
		return fmt.Errorf("wait for poweroff: %w", err)
	}

	return nil
}

func (c *Client) PowerOnVM(ctx context.Context, hetznerID int64) error {
	server, _, err := c.client.Server.GetByID(ctx, hetznerID)
	if err != nil {
		return fmt.Errorf("get server: %w", err)
	}

	action, _, err := c.client.Server.Poweron(ctx, server)
	if err != nil {
		return fmt.Errorf("poweron server: %w", err)
	}

	if err := c.waitForAction(ctx, action); err != nil {
		return fmt.Errorf("wait for poweron: %w", err)
	}

	return nil
}

func (c *Client) waitForAction(ctx context.Context, action *hcloud.Action) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.NewTimer(5 * time.Minute)
	defer timeout.Stop()

	for {
		select {
		case <-ticker.C:
			a, _, err := c.client.Action.GetByID(ctx, action.ID)
			if err != nil {
				return err
			}

			if a.Status == hcloud.ActionStatusSuccess {
				return nil
			}

			if a.Status == hcloud.ActionStatusError {
				return fmt.Errorf("action failed: %s", a.ErrorMessage)
			}

		case <-timeout.C:
			return fmt.Errorf("timeout waiting for action")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}