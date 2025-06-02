package websocket

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/devtail/gateway/internal/queue"
	"github.com/devtail/gateway/internal/terminal"
	"github.com/devtail/gateway/pkg/protocol"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// UnifiedHandler handles both chat and terminal messages
type UnifiedHandler struct {
	conn            *websocket.Conn
	queue           *queue.MessageQueue
	sessionID       string
	send            chan *protocol.Message
	chatHandler     ChatHandler
	terminalHandler *terminal.Handler
	
	// Terminal output channels
	terminalOutputs map[string]chan *protocol.Message
	terminalMu      sync.RWMutex
	
	// State
	mu              sync.RWMutex
	lastActivity    time.Time
	ctx             context.Context
	cancel          context.CancelFunc
}

// TerminalHandler interface for terminal operations
type TerminalHandler interface {
	HandleTerminalMessage(ctx context.Context, msg *protocol.Message) (<-chan *protocol.Message, error)
}

// NewUnifiedHandler creates a handler that supports both chat and terminal
func NewUnifiedHandler(conn *websocket.Conn, chatHandler ChatHandler, terminalManager *terminal.Manager) *UnifiedHandler {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &UnifiedHandler{
		conn:            conn,
		queue:           queue.NewMessageQueue(1000, 3, 30*time.Second),
		sessionID:       uuid.New().String(),
		send:            make(chan *protocol.Message, 256),
		chatHandler:     chatHandler,
		terminalHandler: terminal.NewHandler(terminalManager),
		terminalOutputs: make(map[string]chan *protocol.Message),
		lastActivity:    time.Now(),
		ctx:             ctx,
		cancel:          cancel,
	}
}

func (h *UnifiedHandler) Run() {
	go h.writePump()
	go h.readPump()
	go h.retryPump()
	
	<-h.ctx.Done()
	
	// Cleanup terminal outputs
	h.terminalMu.Lock()
	for _, ch := range h.terminalOutputs {
		close(ch)
	}
	h.terminalMu.Unlock()
}

func (h *UnifiedHandler) readPump() {
	defer h.cancel()
	
	h.conn.SetReadLimit(maxMessageSize)
	h.conn.SetReadDeadline(time.Now().Add(pongTimeout))
	h.conn.SetPongHandler(func(string) error {
		h.conn.SetReadDeadline(time.Now().Add(pongTimeout))
		return nil
	})

	for {
		var msg protocol.Message
		err := h.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Msg("websocket read error")
			}
			return
		}

		h.updateActivity()
		h.routeMessage(&msg)
	}
}

func (h *UnifiedHandler) routeMessage(msg *protocol.Message) {
	// Route based on message type prefix
	switch {
	case msg.Type == protocol.TypeChat:
		h.handleChat(msg)
	case strings.HasPrefix(string(msg.Type), "terminal_"):
		h.handleTerminal(msg)
	case msg.Type == protocol.TypePing:
		h.sendPong()
	case msg.Type == protocol.TypeReconnect:
		h.handleReconnect(msg)
	case msg.Type == protocol.TypeAck:
		h.handleAck(msg)
	default:
		log.Warn().
			Str("type", string(msg.Type)).
			Str("id", msg.ID).
			Msg("unknown message type")
	}
}

func (h *UnifiedHandler) handleChat(msg *protocol.Message) {
	var chatMsg protocol.ChatMessage
	if err := json.Unmarshal(msg.Payload, &chatMsg); err != nil {
		h.sendError(msg.ID, "invalid_payload", err.Error(), false)
		return
	}

	h.queue.Enqueue(msg)

	replies, err := h.chatHandler.HandleChatMessage(h.ctx, &chatMsg)
	if err != nil {
		h.sendError(msg.ID, "chat_error", err.Error(), true)
		h.queue.Ack(msg.ID)
		return
	}

	go func() {
		for reply := range replies {
			replyData, _ := json.Marshal(reply)
			h.send <- &protocol.Message{
				ID:        uuid.New().String(),
				Type:      protocol.TypeChatStream,
				Timestamp: time.Now(),
				Payload:   replyData,
			}
			
			if reply.Finished {
				h.queue.Ack(msg.ID)
				break
			}
		}
	}()
}

func (h *UnifiedHandler) handleTerminal(msg *protocol.Message) {
	replies, err := h.terminalHandler.HandleTerminalMessage(h.ctx, msg)
	if err != nil {
		h.sendError(msg.ID, "terminal_error", err.Error(), false)
		return
	}

	// Handle terminal creation specially to set up output streaming
	if msg.Type == "terminal_create" {
		go h.handleTerminalOutput(msg.ID, replies)
	} else {
		// For other terminal messages, just forward the replies
		go func() {
			for reply := range replies {
				select {
				case h.send <- reply:
				case <-h.ctx.Done():
					return
				}
			}
		}()
	}
}

func (h *UnifiedHandler) handleTerminalOutput(correlationID string, replies <-chan *protocol.Message) {
	// Create a dedicated channel for this terminal's output
	outputChan := make(chan *protocol.Message, 100)
	
	// Store the channel
	h.terminalMu.Lock()
	h.terminalOutputs[correlationID] = outputChan
	h.terminalMu.Unlock()
	
	defer func() {
		h.terminalMu.Lock()
		delete(h.terminalOutputs, correlationID)
		h.terminalMu.Unlock()
		close(outputChan)
	}()
	
	// Forward replies and watch for terminal ID
	var terminalID string
	for reply := range replies {
		// Extract terminal ID from creation response
		if reply.Type == "terminal_created" {
			var resp struct {
				TerminalID string `json:"terminal_id"`
			}
			if err := json.Unmarshal(reply.Payload, &resp); err == nil {
				terminalID = resp.TerminalID
			}
		}
		
		// Forward the reply
		select {
		case h.send <- reply:
		case <-h.ctx.Done():
			return
		}
		
		// For output messages, continue streaming
		if reply.Type == "terminal_output" && terminalID != "" {
			// This goroutine will continue receiving output
			continue
		}
	}
	
	// Continue streaming output for this terminal
	if terminalID != "" {
		for {
			select {
			case output := <-outputChan:
				select {
				case h.send <- output:
				case <-h.ctx.Done():
					return
				}
			case <-h.ctx.Done():
				return
			}
		}
	}
}

func (h *UnifiedHandler) writePump() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		h.conn.Close()
		h.cancel()
	}()

	for {
		select {
		case message, ok := <-h.send:
			h.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if !ok {
				h.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := h.conn.WriteJSON(message); err != nil {
				log.Error().Err(err).Msg("write error")
				return
			}

		case <-ticker.C:
			h.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			if err := h.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-h.ctx.Done():
			return
		}
	}
}

func (h *UnifiedHandler) retryPump() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			messages := h.queue.CheckRetries()
			for _, msg := range messages {
				select {
				case h.send <- msg:
				case <-h.ctx.Done():
					return
				}
			}
		case <-h.ctx.Done():
			return
		}
	}
}

// Reuse existing helper methods
func (h *UnifiedHandler) handleReconnect(msg *protocol.Message) {
	var reconnect protocol.ReconnectMessage
	if err := json.Unmarshal(msg.Payload, &reconnect); err != nil {
		return
	}

	if reconnect.SessionID != h.sessionID {
		return
	}

	messages := h.queue.GetMessagesAfter(reconnect.LastSeqNum)
	for _, m := range messages {
		select {
		case h.send <- m:
		case <-h.ctx.Done():
			return
		}
	}
}

func (h *UnifiedHandler) handleAck(msg *protocol.Message) {
	var ack protocol.AckMessage
	if err := json.Unmarshal(msg.Payload, &ack); err != nil {
		return
	}
	
	h.queue.Ack(ack.MessageID)
}

func (h *UnifiedHandler) sendPong() {
	pong := &protocol.Message{
		ID:        uuid.New().String(),
		Type:      protocol.TypePong,
		Timestamp: time.Now(),
	}
	
	select {
	case h.send <- pong:
	case <-h.ctx.Done():
	}
}

func (h *UnifiedHandler) sendError(messageID, code, error string, retryable bool) {
	errData, _ := json.Marshal(protocol.ChatError{
		Error:     error,
		Code:      code,
		Retryable: retryable,
	})
	
	errMsg := &protocol.Message{
		ID:        messageID,
		Type:      protocol.TypeChatError,
		Timestamp: time.Now(),
		Payload:   errData,
	}
	
	select {
	case h.send <- errMsg:
	case <-h.ctx.Done():
	}
}

func (h *UnifiedHandler) updateActivity() {
	h.mu.Lock()
	h.lastActivity = time.Now()
	h.mu.Unlock()
}

func (h *UnifiedHandler) GetLastActivity() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastActivity
}