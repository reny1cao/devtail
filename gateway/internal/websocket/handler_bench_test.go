package websocket

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devtail/gateway/pkg/protocol"
	"github.com/gorilla/websocket"
)

// mockChatHandler implements a simple chat handler for benchmarking
type mockChatHandler struct{}

func (m *mockChatHandler) HandleChatMessage(ctx context.Context, msg *protocol.ChatMessage) (<-chan *protocol.ChatReply, error) {
	replies := make(chan *protocol.ChatReply, 1)
	go func() {
		defer close(replies)
		replies <- &protocol.ChatReply{
			Content:  "Mock response to: " + msg.Content,
			Finished: true,
		}
	}()
	return replies, nil
}

func (m *mockChatHandler) Initialize(ctx context.Context) error {
	return nil
}

func (m *mockChatHandler) Close() error {
	return nil
}

func BenchmarkWebSocketHandler(b *testing.B) {
	// Create test server
	server := httptest.NewServer(nil)
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create connection
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			b.Fatalf("dial failed: %v", err)
		}

		// Create handler
		handler := NewHandler(conn, &mockChatHandler{})
		go handler.Run()

		// Send message
		msg := protocol.Message{
			ID:        "test-" + string(rune(i)),
			Type:      protocol.TypeChat,
			Timestamp: time.Now(),
		}
		
		chatMsg := protocol.ChatMessage{
			Role:    "user",
			Content: "benchmark test message",
		}
		
		payload, _ := json.Marshal(chatMsg)
		msg.Payload = payload

		if err := conn.WriteJSON(msg); err != nil {
			b.Errorf("write failed: %v", err)
		}

		// Read response
		var reply protocol.Message
		if err := conn.ReadJSON(&reply); err != nil {
			b.Errorf("read failed: %v", err)
		}

		conn.Close()
	}
}

func BenchmarkMessageSerialization(b *testing.B) {
	msg := protocol.Message{
		ID:        "bench-123",
		Type:      protocol.TypeChat,
		Timestamp: time.Now(),
		SeqNum:    42,
	}
	
	chatMsg := protocol.ChatMessage{
		Role:    "user",
		Content: "This is a benchmark test message with some content",
	}
	
	payload, _ := json.Marshal(chatMsg)
	msg.Payload = payload

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := json.Marshal(msg)
		if err != nil {
			b.Fatal(err)
		}
		
		var decoded protocol.Message
		if err := json.Unmarshal(data, &decoded); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkConcurrentConnections(b *testing.B) {
	// Create test server with handler
	server := httptest.NewServer(nil)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	b.ResetTimer()

	// Run N concurrent connections
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				b.Errorf("dial failed: %v", err)
				continue
			}

			// Simulate some activity
			for j := 0; j < 10; j++ {
				msg := protocol.Message{
					ID:        "test",
					Type:      protocol.TypePing,
					Timestamp: time.Now(),
				}
				
				conn.WriteJSON(msg)
				
				var reply protocol.Message
				conn.ReadJSON(&reply)
			}

			conn.Close()
		}
	})
}

func BenchmarkMessageQueue(b *testing.B) {
	// This would benchmark the message queue performance
	// Implementation depends on queue package details
}

func BenchmarkStreamingResponse(b *testing.B) {
	handler := &mockChatHandler{}
	ctx := context.Background()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		msg := &protocol.ChatMessage{
			Role:    "user",
			Content: "Generate a long response",
		}

		replies, err := handler.HandleChatMessage(ctx, msg)
		if err != nil {
			b.Fatal(err)
		}

		// Consume all replies
		for range replies {
		}
	}
}