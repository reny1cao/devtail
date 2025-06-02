package protocol

import (
	"encoding/json"
	"time"
)

type MessageType string

const (
	TypeChat       MessageType = "chat"
	TypeChatReply  MessageType = "chat_reply"
	TypeChatStream MessageType = "chat_stream"
	TypeChatError  MessageType = "chat_error"
	TypePing       MessageType = "ping"
	TypePong       MessageType = "pong"
	TypeReconnect  MessageType = "reconnect"
	TypeAck        MessageType = "ack"
)

type Message struct {
	ID            string          `json:"id"`
	Type          MessageType     `json:"type"`
	Timestamp     time.Time       `json:"timestamp"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	SeqNum        uint64          `json:"seq_num,omitempty"`
	RequiresAck   bool            `json:"requires_ack,omitempty"`
	RetryCount    int             `json:"retry_count,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatReply struct {
	Content  string `json:"content"`
	Finished bool   `json:"finished"`
}

type ChatError struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Retryable bool `json:"retryable"`
}

type ReconnectMessage struct {
	LastSeqNum uint64 `json:"last_seq_num"`
	SessionID  string `json:"session_id"`
}

type AckMessage struct {
	MessageID string `json:"message_id"`
	SeqNum    uint64 `json:"seq_num"`
}

// Now returns the current time for use in messages
func Now() time.Time {
	return time.Now()
}