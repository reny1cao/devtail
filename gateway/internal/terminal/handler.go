package terminal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/devtail/gateway/pkg/protocol"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Handler integrates terminals with WebSocket messaging
type Handler struct {
	manager *Manager
}

// NewHandler creates a new terminal handler
func NewHandler(manager *Manager) *Handler {
	return &Handler{
		manager: manager,
	}
}

// HandleTerminalMessage processes terminal-related messages
func (h *Handler) HandleTerminalMessage(ctx context.Context, msg *protocol.Message) (<-chan *protocol.Message, error) {
	replies := make(chan *protocol.Message, 10)
	
	go func() {
		defer close(replies)
		
		switch msg.Type {
		case "terminal_create":
			h.handleCreate(ctx, msg, replies)
		case "terminal_input":
			h.handleInput(ctx, msg, replies)
		case "terminal_resize":
			h.handleResize(ctx, msg, replies)
		case "terminal_close":
			h.handleClose(ctx, msg, replies)
		case "terminal_list":
			h.handleList(ctx, msg, replies)
		default:
			h.sendError(replies, msg.ID, "Unknown terminal message type")
		}
	}()
	
	return replies, nil
}

// Message types

type TerminalCreateRequest struct {
	WorkDir string   `json:"work_dir,omitempty"`
	Env     []string `json:"env,omitempty"`
	Rows    uint16   `json:"rows,omitempty"`
	Cols    uint16   `json:"cols,omitempty"`
}

type TerminalCreateResponse struct {
	TerminalID string `json:"terminal_id"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
}

type TerminalInputMessage struct {
	TerminalID string `json:"terminal_id"`
	Data       string `json:"data"` // base64 encoded
}

type TerminalOutputMessage struct {
	TerminalID string `json:"terminal_id"`
	Data       string `json:"data"` // base64 encoded
	Stderr     bool   `json:"stderr,omitempty"`
}

type TerminalResizeMessage struct {
	TerminalID string `json:"terminal_id"`
	Rows       uint16 `json:"rows"`
	Cols       uint16 `json:"cols"`
}

// Handlers

func (h *Handler) handleCreate(ctx context.Context, msg *protocol.Message, replies chan<- *protocol.Message) {
	var req TerminalCreateRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		h.sendError(replies, msg.ID, "Invalid create request")
		return
	}
	
	// Set defaults
	if req.Rows == 0 {
		req.Rows = 24
	}
	if req.Cols == 0 {
		req.Cols = 80
	}
	
	// Create terminal
	term, err := h.manager.CreateTerminal(req.WorkDir, req.Env)
	if err != nil {
		h.sendError(replies, msg.ID, fmt.Sprintf("Failed to create terminal: %v", err))
		return
	}
	
	// Set initial size
	if err := term.Resize(req.Rows, req.Cols); err != nil {
		log.Error().Err(err).Msg("failed to set initial terminal size")
	}
	
	// Send success response
	resp := TerminalCreateResponse{
		TerminalID: term.ID,
		Success:    true,
	}
	
	respData, _ := json.Marshal(resp)
	replies <- &protocol.Message{
		ID:            msg.ID,
		Type:          "terminal_created",
		Timestamp:     msg.Timestamp,
		Payload:       respData,
		CorrelationID: msg.ID,
	}
	
	// Start output streaming
	go h.streamOutput(ctx, term, replies)
}

func (h *Handler) handleInput(ctx context.Context, msg *protocol.Message, replies chan<- *protocol.Message) {
	var input TerminalInputMessage
	if err := json.Unmarshal(msg.Payload, &input); err != nil {
		h.sendError(replies, msg.ID, "Invalid input message")
		return
	}
	
	// Get terminal
	term, err := h.manager.GetTerminal(input.TerminalID)
	if err != nil {
		h.sendError(replies, msg.ID, fmt.Sprintf("Terminal not found: %v", err))
		return
	}
	
	// Decode input data
	data, err := base64.StdEncoding.DecodeString(input.Data)
	if err != nil {
		h.sendError(replies, msg.ID, "Invalid base64 input")
		return
	}
	
	// Write to terminal
	if err := term.Write(data); err != nil {
		h.sendError(replies, msg.ID, fmt.Sprintf("Write failed: %v", err))
		return
	}
	
	// Send ACK
	h.sendAck(replies, msg.ID)
}

func (h *Handler) handleResize(ctx context.Context, msg *protocol.Message, replies chan<- *protocol.Message) {
	var resize TerminalResizeMessage
	if err := json.Unmarshal(msg.Payload, &resize); err != nil {
		h.sendError(replies, msg.ID, "Invalid resize message")
		return
	}
	
	// Get terminal
	term, err := h.manager.GetTerminal(resize.TerminalID)
	if err != nil {
		h.sendError(replies, msg.ID, fmt.Sprintf("Terminal not found: %v", err))
		return
	}
	
	// Resize terminal
	if err := term.Resize(resize.Rows, resize.Cols); err != nil {
		h.sendError(replies, msg.ID, fmt.Sprintf("Resize failed: %v", err))
		return
	}
	
	// Send ACK
	h.sendAck(replies, msg.ID)
	
	log.Debug().
		Str("terminal_id", resize.TerminalID).
		Uint16("rows", resize.Rows).
		Uint16("cols", resize.Cols).
		Msg("terminal resized")
}

func (h *Handler) handleClose(ctx context.Context, msg *protocol.Message, replies chan<- *protocol.Message) {
	var req struct {
		TerminalID string `json:"terminal_id"`
	}
	
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		h.sendError(replies, msg.ID, "Invalid close request")
		return
	}
	
	// Close terminal
	if err := h.manager.CloseTerminal(req.TerminalID); err != nil {
		h.sendError(replies, msg.ID, fmt.Sprintf("Close failed: %v", err))
		return
	}
	
	// Send ACK
	h.sendAck(replies, msg.ID)
}

func (h *Handler) handleList(ctx context.Context, msg *protocol.Message, replies chan<- *protocol.Message) {
	terminals := h.manager.ListTerminals()
	stats := h.manager.GetStats()
	
	resp := map[string]interface{}{
		"terminals": terminals,
		"stats":     stats,
	}
	
	respData, _ := json.Marshal(resp)
	replies <- &protocol.Message{
		ID:            uuid.New().String(),
		Type:          "terminal_list",
		Timestamp:     msg.Timestamp,
		Payload:       respData,
		CorrelationID: msg.ID,
	}
}

// streamOutput continuously sends terminal output to the client
func (h *Handler) streamOutput(ctx context.Context, term *Terminal, replies chan<- *protocol.Message) {
	outputChan := term.Read()
	
	for {
		select {
		case data, ok := <-outputChan:
			if !ok {
				// Terminal closed
				return
			}
			
			// Send output message
			output := TerminalOutputMessage{
				TerminalID: term.ID,
				Data:       base64.StdEncoding.EncodeToString(data),
				Stderr:     false,
			}
			
			outputData, _ := json.Marshal(output)
			
			select {
			case replies <- &protocol.Message{
				ID:        uuid.New().String(),
				Type:      "terminal_output",
				Timestamp: protocol.Now(),
				Payload:   outputData,
			}:
			case <-ctx.Done():
				return
			}
			
		case <-ctx.Done():
			return
		}
	}
}

// Helper methods

func (h *Handler) sendError(replies chan<- *protocol.Message, correlationID, error string) {
	errData, _ := json.Marshal(map[string]string{
		"error": error,
	})
	
	replies <- &protocol.Message{
		ID:            uuid.New().String(),
		Type:          "terminal_error",
		Timestamp:     protocol.Now(),
		Payload:       errData,
		CorrelationID: correlationID,
	}
}

func (h *Handler) sendAck(replies chan<- *protocol.Message, messageID string) {
	ackData, _ := json.Marshal(map[string]interface{}{
		"message_id": messageID,
		"success":    true,
	})
	
	replies <- &protocol.Message{
		ID:        uuid.New().String(),
		Type:      protocol.TypeAck,
		Timestamp: protocol.Now(),
		Payload:   ackData,
	}
}