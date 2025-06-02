package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/devtail/gateway/pkg/protocol"
)

func TestMockAiderHandler(t *testing.T) {
	handler := NewAiderHandler(".")
	defer handler.Close()

	ctx := context.Background()
	msg := &protocol.ChatMessage{
		Role:    "user",
		Content: "Write a hello world function",
	}

	replies, err := handler.HandleChatMessage(ctx, msg)
	if err != nil {
		t.Fatalf("HandleChatMessage failed: %v", err)
	}

	var response strings.Builder
	finished := false

	for reply := range replies {
		response.WriteString(reply.Content)
		if reply.Finished {
			finished = true
		}
	}

	if !finished {
		t.Error("Expected finished flag to be set")
	}

	if !strings.Contains(response.String(), "hello world") {
		t.Errorf("Expected response to contain 'hello world', got: %s", response.String())
	}
}

func TestAiderHandlerTimeout(t *testing.T) {
	handler := NewAiderHandler(".")
	defer handler.Close()

	// Create a context that times out quickly
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	msg := &protocol.ChatMessage{
		Role:    "user",
		Content: "test",
	}

	replies, err := handler.HandleChatMessage(ctx, msg)
	if err != nil {
		t.Fatalf("HandleChatMessage failed: %v", err)
	}

	// Consume replies until context cancels
	for range replies {
		// Just drain the channel
	}
}

func TestAiderHandlerConcurrent(t *testing.T) {
	handler := NewAiderHandler(".")
	defer handler.Close()

	ctx := context.Background()
	
	// Run multiple concurrent requests
	for i := 0; i < 5; i++ {
		go func(n int) {
			msg := &protocol.ChatMessage{
				Role:    "user",
				Content: "test message",
			}

			replies, err := handler.HandleChatMessage(ctx, msg)
			if err != nil {
				t.Errorf("HandleChatMessage %d failed: %v", n, err)
				return
			}

			// Consume all replies
			for range replies {
			}
		}(i)
	}

	// Give goroutines time to complete
	time.Sleep(2 * time.Second)
}

func TestRealAiderConfig(t *testing.T) {
	config := AiderConfig{
		Model:      "gpt-3.5-turbo",
		YesAlways:  true,
		NoGit:      true,
		EditFormat: "diff",
	}

	handler := NewRealAiderHandler(".", config)
	
	// Test that buildAiderArgs produces correct arguments
	args := handler.buildAiderArgs()
	
	expectedArgs := map[string]bool{
		"--model":       true,
		"--yes-always":  true,
		"--no-git":      true,
		"--edit-format": true,
		"--no-pretty":   true,
		"--no-stream":   true,
	}

	for _, arg := range args {
		if _, ok := expectedArgs[arg]; ok {
			delete(expectedArgs, arg)
		}
	}

	if len(expectedArgs) > 0 {
		t.Errorf("Missing expected arguments: %v", expectedArgs)
	}
}

func TestFactoryMockMode(t *testing.T) {
	// Test that factory creates mock handler when requested
	handler := NewHandler(".", true)
	
	// Check it's the mock implementation
	if _, ok := handler.(*AiderHandler); !ok {
		t.Error("Expected mock AiderHandler")
	}
}

func TestFactoryRealMode(t *testing.T) {
	// Test that factory creates real handler when requested
	handler := NewHandler(".", false)
	
	// Check it's the real implementation
	if _, ok := handler.(*RealAiderHandler); !ok {
		t.Error("Expected real AiderHandler")
	}
}