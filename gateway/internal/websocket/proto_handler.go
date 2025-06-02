package websocket

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/devtail/gateway/internal/queue"
	"github.com/devtail/gateway/pkg/protocol"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// ProtoHandler handles WebSocket connections using Protocol Buffers
type ProtoHandler struct {
	conn         *websocket.Conn
	codec        *protocol.Codec
	queue        *queue.MessageQueue
	sessionID    string
	chatHandler  ChatHandler
	
	// Channels
	send         chan *protocol.Message
	sendBatch    chan []*protocol.Message
	
	// State
	mu           sync.RWMutex
	lastActivity time.Time
	seqNum       uint64
	
	// Lifecycle
	ctx          context.Context
	cancel       context.CancelFunc
	
	// Options
	batchSize    int
	batchTimeout time.Duration
	useBinary    bool
}

// ProtoHandlerOption configures the handler
type ProtoHandlerOption func(*ProtoHandler)

// WithBatching enables message batching
func WithBatching(size int, timeout time.Duration) ProtoHandlerOption {
	return func(h *ProtoHandler) {
		h.batchSize = size
		h.batchTimeout = timeout
	}
}

// WithBinaryFrames uses binary WebSocket frames (more efficient)
func WithBinaryFrames() ProtoHandlerOption {
	return func(h *ProtoHandler) {
		h.useBinary = true
	}
}

// NewProtoHandler creates a new Protocol Buffer WebSocket handler
func NewProtoHandler(conn *websocket.Conn, chatHandler ChatHandler, opts ...ProtoHandlerOption) (*ProtoHandler, error) {
	codec, err := protocol.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("create codec: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	
	h := &ProtoHandler{
		conn:         conn,
		codec:        codec,
		queue:        queue.NewMessageQueue(1000, 3, 30*time.Second),
		sessionID:    uuid.New().String(),
		chatHandler:  chatHandler,
		send:         make(chan *protocol.Message, 256),
		sendBatch:    make(chan []*protocol.Message, 32),
		lastActivity: time.Now(),
		ctx:          ctx,
		cancel:       cancel,
		batchSize:    10,
		batchTimeout: 50 * time.Millisecond,
		useBinary:    false,
	}

	// Apply options
	for _, opt := range opts {
		opt(h)
	}

	return h, nil
}

// Run starts the handler loops
func (h *ProtoHandler) Run() {
	// Start goroutines
	go h.readPump()
	go h.writePump()
	go h.retryPump()
	
	if h.batchSize > 1 {
		go h.batchPump()
	}

	// Send session start
	h.sendSessionStart()

	// Wait for shutdown
	<-h.ctx.Done()
}

func (h *ProtoHandler) readPump() {
	defer h.cancel()
	
	h.conn.SetReadLimit(maxMessageSize)
	h.conn.SetReadDeadline(time.Now().Add(pongTimeout))
	h.conn.SetPongHandler(func(string) error {
		h.conn.SetReadDeadline(time.Now().Add(pongTimeout))
		return nil
	})

	for {
		messageType, data, err := h.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Error().Err(err).Msg("websocket read error")
			}
			return
		}

		// Only accept binary frames when using Protocol Buffers
		if h.useBinary && messageType != websocket.BinaryMessage {
			log.Warn().Int("type", messageType).Msg("expected binary frame")
			continue
		}

		// Decode message
		msg, err := h.codec.DecodeMessage(data)
		if err != nil {
			log.Error().Err(err).Msg("decode message failed")
			continue
		}

		h.updateActivity()
		h.handleMessage(msg)
	}
}

func (h *ProtoHandler) writePump() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		h.conn.Close()
		h.cancel()
	}()

	for {
		select {
		case message := <-h.send:
			if err := h.writeMessage(message); err != nil {
				log.Error().Err(err).Msg("write message failed")
				return
			}

		case batch := <-h.sendBatch:
			if err := h.writeBatch(batch); err != nil {
				log.Error().Err(err).Msg("write batch failed")
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

func (h *ProtoHandler) batchPump() {
	ticker := time.NewTicker(h.batchTimeout)
	defer ticker.Stop()

	batch := make([]*protocol.Message, 0, h.batchSize)

	for {
		select {
		case msg := <-h.send:
			batch = append(batch, msg)
			
			if len(batch) >= h.batchSize {
				h.sendBatch <- batch
				batch = make([]*protocol.Message, 0, h.batchSize)
				ticker.Reset(h.batchTimeout)
			}

		case <-ticker.C:
			if len(batch) > 0 {
				h.sendBatch <- batch
				batch = make([]*protocol.Message, 0, h.batchSize)
			}

		case <-h.ctx.Done():
			return
		}
	}
}

func (h *ProtoHandler) retryPump() {
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

func (h *ProtoHandler) writeMessage(msg *protocol.Message) error {
	// Set sequence number
	msg.SeqNum = h.nextSeqNum()

	// Encode message
	data, err := h.codec.EncodeMessage(msg)
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}

	// Set write deadline
	h.conn.SetWriteDeadline(time.Now().Add(writeTimeout))

	// Write frame
	messageType := websocket.TextMessage
	if h.useBinary {
		messageType = websocket.BinaryMessage
	}

	return h.conn.WriteMessage(messageType, data)
}

func (h *ProtoHandler) writeBatch(messages []*protocol.Message) error {
	// Set sequence numbers
	for _, msg := range messages {
		msg.SeqNum = h.nextSeqNum()
	}

	// Encode batch
	data, err := h.codec.EncodeBatch(messages)
	if err != nil {
		return fmt.Errorf("encode batch: %w", err)
	}

	// Set write deadline
	h.conn.SetWriteDeadline(time.Now().Add(writeTimeout))

	// Write frame (batches are always binary)
	return h.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (h *ProtoHandler) handleMessage(msg *protocol.Message) {
	switch msg.Type {
	case protocol.TypeChat:
		h.handleChat(msg)
	case protocol.TypePing:
		h.sendPong()
	case protocol.TypeReconnect:
		h.handleReconnect(msg)
	case protocol.TypeAck:
		h.handleAck(msg)
	default:
		log.Warn().Str("type", string(msg.Type)).Msg("unknown message type")
	}
}

func (h *ProtoHandler) sendSessionStart() {
	// This would send session start with client capabilities
	msg := &protocol.Message{
		ID:        uuid.New().String(),
		Type:      "session_start",
		Timestamp: time.Now(),
		Payload:   []byte(fmt.Sprintf(`{"session_id":"%s"}`, h.sessionID)),
	}

	select {
	case h.send <- msg:
	case <-h.ctx.Done():
	}
}

func (h *ProtoHandler) nextSeqNum() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seqNum++
	return h.seqNum
}

func (h *ProtoHandler) updateActivity() {
	h.mu.Lock()
	h.lastActivity = time.Now()
	h.mu.Unlock()
}

// Reuse existing handler methods for chat, reconnect, ack, etc.
// These would be the same as in the original handler

func (h *ProtoHandler) handleChat(msg *protocol.Message) {
	// Implementation same as original Handler
}

func (h *ProtoHandler) sendPong() {
	// Implementation same as original Handler
}

func (h *ProtoHandler) handleReconnect(msg *protocol.Message) {
	// Implementation same as original Handler
}

func (h *ProtoHandler) handleAck(msg *protocol.Message) {
	// Implementation same as original Handler
}