package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/devtail/gateway/internal/queue"
	"github.com/devtail/gateway/pkg/protocol"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	writeTimeout   = 10 * time.Second
	pongTimeout    = 60 * time.Second
	pingInterval   = 30 * time.Second
	maxMessageSize = 65536
)

type Handler struct {
	conn         *websocket.Conn
	queue        *queue.MessageQueue
	sessionID    string
	send         chan *protocol.Message
	chatHandler  ChatHandler
	mu           sync.RWMutex
	lastActivity time.Time
	ctx          context.Context
	cancel       context.CancelFunc
}

type ChatHandler interface {
	HandleChatMessage(ctx context.Context, msg *protocol.ChatMessage) (<-chan *protocol.ChatReply, error)
}

func NewHandler(conn *websocket.Conn, chatHandler ChatHandler) *Handler {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Handler{
		conn:         conn,
		queue:        queue.NewMessageQueue(1000, 3, 30*time.Second),
		sessionID:    uuid.New().String(),
		send:         make(chan *protocol.Message, 256),
		chatHandler:  chatHandler,
		lastActivity: time.Now(),
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (h *Handler) Run() {
	go h.writePump()
	go h.readPump()
	go h.retryPump()
	
	<-h.ctx.Done()
}

func (h *Handler) readPump() {
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

		switch msg.Type {
		case protocol.TypeChat:
			h.handleChat(&msg)
		case protocol.TypePing:
			h.sendPong()
		case protocol.TypeReconnect:
			h.handleReconnect(&msg)
		case protocol.TypeAck:
			h.handleAck(&msg)
		}
	}
}

func (h *Handler) writePump() {
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

func (h *Handler) retryPump() {
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

func (h *Handler) handleChat(msg *protocol.Message) {
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

func (h *Handler) handleReconnect(msg *protocol.Message) {
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

func (h *Handler) handleAck(msg *protocol.Message) {
	var ack protocol.AckMessage
	if err := json.Unmarshal(msg.Payload, &ack); err != nil {
		return
	}
	
	h.queue.Ack(ack.MessageID)
}

func (h *Handler) sendPong() {
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

func (h *Handler) sendError(messageID, code, error string, retryable bool) {
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

func (h *Handler) updateActivity() {
	h.mu.Lock()
	h.lastActivity = time.Now()
	h.mu.Unlock()
}

func (h *Handler) GetLastActivity() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastActivity
}