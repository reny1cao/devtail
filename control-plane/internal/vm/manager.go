package vm

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/devtail/control-plane/internal/hetzner"
	"github.com/devtail/control-plane/internal/tailscale"
	"github.com/devtail/control-plane/pkg/models"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

type Manager struct {
	db             *sql.DB
	hetznerClient  *hetzner.Client
	tailscaleClient *tailscale.Client
	config         Config
}

type Config struct {
	SSHPublicKey string
	GatewayURL   string
	CallbackURL  string
	WebSocketBaseURL string
}

func NewManager(db *sql.DB, hetznerClient *hetzner.Client, tailscaleClient *tailscale.Client, config Config) *Manager {
	return &Manager{
		db:              db,
		hetznerClient:   hetznerClient,
		tailscaleClient: tailscaleClient,
		config:          config,
	}
}

func (m *Manager) CreateVM(ctx context.Context, req *models.CreateVMRequest) (*models.CreateVMResponse, error) {
	// Create VM record
	vm := &models.VM{
		ID:             uuid.New().String(),
		UserID:         req.UserID,
		Status:         models.VMStatusProvisioning,
		Spec:           req.Spec,
		WebsocketToken: m.generateToken(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Start transaction
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert VM record
	if err := m.insertVM(ctx, tx, vm); err != nil {
		return nil, fmt.Errorf("insert vm: %w", err)
	}

	// Commit early to make VM visible
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	// Start async provisioning
	go m.provisionVM(context.Background(), vm)

	return &models.CreateVMResponse{
		VM:             vm,
		WebsocketURL:   fmt.Sprintf("%s/ws?token=%s", m.config.WebSocketBaseURL, vm.WebsocketToken),
		EstimatedReady: 60,
	}, nil
}

func (m *Manager) provisionVM(ctx context.Context, vm *models.VM) {
	log.Info().Str("vm_id", vm.ID).Msg("Starting VM provisioning")

	// Create Tailscale auth key
	authKey, err := m.tailscaleClient.CreateAuthKey(ctx, fmt.Sprintf("devtail-%s", vm.ID))
	if err != nil {
		log.Error().Err(err).Str("vm_id", vm.ID).Msg("Failed to create Tailscale auth key")
		m.updateVMStatus(ctx, vm.ID, models.VMStatusError)
		return
	}

	vm.TailscaleAuthKey = authKey.Key

	// Generate cloud-init script
	cloudInit, err := GenerateCloudInit(CloudInitData{
		VMID:             vm.ID,
		TailscaleAuthKey: authKey.Key,
		SSHPublicKey:     m.config.SSHPublicKey,
		GatewayURL:       m.config.GatewayURL,
		CallbackURL:      m.config.CallbackURL,
	})
	if err != nil {
		log.Error().Err(err).Str("vm_id", vm.ID).Msg("Failed to generate cloud-init")
		m.updateVMStatus(ctx, vm.ID, models.VMStatusError)
		return
	}

	// Create Hetzner VM
	if err := m.hetznerClient.CreateVM(ctx, vm, cloudInit); err != nil {
		log.Error().Err(err).Str("vm_id", vm.ID).Msg("Failed to create Hetzner VM")
		m.updateVMStatus(ctx, vm.ID, models.VMStatusError)
		return
	}

	// Update VM with Hetzner ID
	if err := m.updateVMHetznerID(ctx, vm.ID, vm.HetznerID); err != nil {
		log.Error().Err(err).Str("vm_id", vm.ID).Msg("Failed to update VM Hetzner ID")
		return
	}

	// Wait for Tailscale device to appear
	device, err := m.tailscaleClient.WaitForDevice(ctx, fmt.Sprintf("devtail-%s", vm.ID), 5*time.Minute)
	if err != nil {
		log.Error().Err(err).Str("vm_id", vm.ID).Msg("Failed to wait for Tailscale device")
		m.updateVMStatus(ctx, vm.ID, models.VMStatusError)
		return
	}

	// Extract Tailscale IP
	if len(device.Addresses) == 0 {
		log.Error().Str("vm_id", vm.ID).Msg("No Tailscale addresses found")
		m.updateVMStatus(ctx, vm.ID, models.VMStatusError)
		return
	}

	vm.TailscaleIP = device.Addresses[0]

	// Update VM with Tailscale IP and mark as running
	if err := m.updateVMReady(ctx, vm.ID, vm.TailscaleIP); err != nil {
		log.Error().Err(err).Str("vm_id", vm.ID).Msg("Failed to update VM as ready")
		return
	}

	log.Info().
		Str("vm_id", vm.ID).
		Str("tailscale_ip", vm.TailscaleIP).
		Msg("VM provisioning completed")
}

func (m *Manager) generateToken() string {
	token := uuid.New().String()
	hash, _ := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	return string(hash)
}

func (m *Manager) insertVM(ctx context.Context, tx *sql.Tx, vm *models.VM) error {
	query := `
		INSERT INTO vms (
			id, user_id, status, spec, websocket_token, 
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	
	specJSON, err := json.Marshal(vm.Spec)
	if err != nil {
		return fmt.Errorf("marshal spec: %w", err)
	}

	_, err = tx.ExecContext(ctx, query,
		vm.ID, vm.UserID, vm.Status, specJSON, vm.WebsocketToken,
		vm.CreatedAt, vm.UpdatedAt,
	)
	return err
}

func (m *Manager) updateVMStatus(ctx context.Context, vmID string, status models.VMStatus) error {
	query := `UPDATE vms SET status = $1, updated_at = $2 WHERE id = $3`
	_, err := m.db.ExecContext(ctx, query, status, time.Now(), vmID)
	return err
}

func (m *Manager) updateVMHetznerID(ctx context.Context, vmID string, hetznerID int64) error {
	query := `UPDATE vms SET hetzner_id = $1, updated_at = $2 WHERE id = $3`
	_, err := m.db.ExecContext(ctx, query, hetznerID, time.Now(), vmID)
	return err
}

func (m *Manager) updateVMReady(ctx context.Context, vmID string, tailscaleIP string) error {
	query := `
		UPDATE vms 
		SET status = $1, tailscale_ip = $2, updated_at = $3 
		WHERE id = $4
	`
	_, err := m.db.ExecContext(ctx, query, 
		models.VMStatusRunning, tailscaleIP, time.Now(), vmID,
	)
	return err
}

func (m *Manager) GetVM(ctx context.Context, vmID string) (*models.VM, error) {
	query := `
		SELECT id, user_id, hetzner_id, tailscale_ip, status, spec,
		       websocket_token, last_activity, created_at, updated_at
		FROM vms
		WHERE id = $1
	`

	var vm models.VM
	var specJSON []byte

	err := m.db.QueryRowContext(ctx, query, vmID).Scan(
		&vm.ID, &vm.UserID, &vm.HetznerID, &vm.TailscaleIP,
		&vm.Status, &specJSON, &vm.WebsocketToken,
		&vm.LastActivity, &vm.CreatedAt, &vm.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(specJSON, &vm.Spec); err != nil {
		return nil, fmt.Errorf("unmarshal spec: %w", err)
	}

	return &vm, nil
}

func (m *Manager) DeleteVM(ctx context.Context, vmID string) error {
	vm, err := m.GetVM(ctx, vmID)
	if err != nil {
		return fmt.Errorf("get vm: %w", err)
	}

	// Delete from Hetzner
	if vm.HetznerID != 0 {
		if err := m.hetznerClient.DeleteVM(ctx, vm.HetznerID); err != nil {
			log.Error().Err(err).Str("vm_id", vmID).Msg("Failed to delete Hetzner VM")
		}
	}

	// Update status to terminated
	return m.updateVMStatus(ctx, vmID, models.VMStatusTerminated)
}