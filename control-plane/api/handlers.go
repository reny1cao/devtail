package api

import (
	"net/http"

	"github.com/devtail/control-plane/internal/vm"
	"github.com/devtail/control-plane/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

type Handlers struct {
	vmManager *vm.Manager
}

func NewHandlers(vmManager *vm.Manager) *Handlers {
	return &Handlers{
		vmManager: vmManager,
	}
}

func (h *Handlers) CreateVM(c *gin.Context) {
	var req models.CreateVMRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user ID from auth context (simplified for now)
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing user ID"})
		return
	}
	req.UserID = userID

	resp, err := h.vmManager.CreateVM(c.Request.Context(), &req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create VM")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create VM"})
		return
	}

	c.JSON(http.StatusCreated, resp)
}

func (h *Handlers) GetVM(c *gin.Context) {
	vmID := c.Param("id")
	
	vm, err := h.vmManager.GetVM(c.Request.Context(), vmID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "VM not found"})
		return
	}

	// Check user authorization
	userID := c.GetHeader("X-User-ID")
	if vm.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	c.JSON(http.StatusOK, vm)
}

func (h *Handlers) DeleteVM(c *gin.Context) {
	vmID := c.Param("id")
	
	vm, err := h.vmManager.GetVM(c.Request.Context(), vmID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "VM not found"})
		return
	}

	// Check user authorization
	userID := c.GetHeader("X-User-ID")
	if vm.UserID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := h.vmManager.DeleteVM(c.Request.Context(), vmID); err != nil {
		log.Error().Err(err).Msg("Failed to delete VM")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete VM"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (h *Handlers) VMCallback(c *gin.Context) {
	var callback struct {
		VMID        string `json:"vm_id"`
		TailscaleIP string `json:"tailscale_ip"`
		Status      string `json:"status"`
	}

	if err := c.ShouldBindJSON(&callback); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Info().
		Str("vm_id", callback.VMID).
		Str("tailscale_ip", callback.TailscaleIP).
		Str("status", callback.Status).
		Msg("VM callback received")

	// In production, you'd update the VM status based on the callback
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handlers) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"service": "control-plane",
	})
}