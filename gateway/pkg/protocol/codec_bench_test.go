package protocol

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func BenchmarkJSONEncoding(b *testing.B) {
	msg := &Message{
		ID:        uuid.New().String(),
		Type:      TypeChat,
		Timestamp: time.Now(),
		SeqNum:    42,
		Payload: []byte(`{
			"role": "user",
			"content": "Write a function that calculates the fibonacci sequence up to n terms",
			"metadata": {
				"model": "claude-3-sonnet",
				"temperature": "0.7"
			}
		}`),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		data, err := json.Marshal(msg)
		if err != nil {
			b.Fatal(err)
		}
		
		var decoded Message
		if err := json.Unmarshal(data, &decoded); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProtobufEncoding(b *testing.B) {
	codec, err := NewCodec()
	if err != nil {
		b.Fatal(err)
	}

	msg := &Message{
		ID:        uuid.New().String(),
		Type:      TypeChat,
		Timestamp: time.Now(),
		SeqNum:    42,
		Payload: []byte(`{
			"role": "user",
			"content": "Write a function that calculates the fibonacci sequence up to n terms",
			"metadata": {
				"model": "claude-3-sonnet",
				"temperature": "0.7"
			}
		}`),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		data, err := codec.EncodeMessage(msg)
		if err != nil {
			b.Fatal(err)
		}
		
		decoded, err := codec.DecodeMessage(data)
		if err != nil {
			b.Fatal(err)
		}
		_ = decoded
	}
}

func BenchmarkBatchEncoding(b *testing.B) {
	codec, err := NewCodec()
	if err != nil {
		b.Fatal(err)
	}

	// Create 10 messages
	messages := make([]*Message, 10)
	for i := range messages {
		messages[i] = &Message{
			ID:        uuid.New().String(),
			Type:      TypeChatStream,
			Timestamp: time.Now(),
			SeqNum:    uint64(i),
			Payload:   []byte(`{"content": "This is a streaming response token.", "finished": false}`),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		data, err := codec.EncodeBatch(messages)
		if err != nil {
			b.Fatal(err)
		}
		_ = data
	}
}

func BenchmarkCompressionLargeMessage(b *testing.B) {
	codec, err := NewCodec()
	if err != nil {
		b.Fatal(err)
	}

	// Create a large message that will benefit from compression
	largeContent := make([]byte, 10000)
	for i := range largeContent {
		largeContent[i] = byte('A' + (i % 26))
	}

	msg := &Message{
		ID:        uuid.New().String(),
		Type:      TypeChat,
		Timestamp: time.Now(),
		Payload:   largeContent,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		data, err := codec.EncodeMessage(msg)
		if err != nil {
			b.Fatal(err)
		}
		
		decoded, err := codec.DecodeMessage(data)
		if err != nil {
			b.Fatal(err)
		}
		_ = decoded
	}
	
	// Report compression ratio
	uncompressed, _ := json.Marshal(msg)
	compressed, _ := codec.EncodeMessage(msg)
	b.Logf("Compression ratio: %.2f%% (from %d to %d bytes)", 
		float64(len(compressed))*100/float64(len(uncompressed)),
		len(uncompressed), len(compressed))
}

func BenchmarkStreamingMessages(b *testing.B) {
	codec, err := NewCodec()
	if err != nil {
		b.Fatal(err)
	}

	// Simulate streaming chat tokens
	tokens := []string{
		"Here", "'s", " a", " function", " to", " calculate", " the", " Fibonacci",
		" sequence", ":", "\n\n", "```python", "\ndef", " fibonacci", "(n):",
		"\n    ", "if", " n", " <=", " 0:", "\n        ", "return", " []",
	}

	messages := make([]*Message, len(tokens))
	for i, token := range tokens {
		messages[i] = &Message{
			ID:        uuid.New().String(),
			Type:      TypeChatStream,
			Timestamp: time.Now(),
			SeqNum:    uint64(i),
			Payload:   []byte(`{"content": "` + token + `", "finished": false}`),
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for _, msg := range messages {
			data, err := codec.EncodeMessage(msg)
			if err != nil {
				b.Fatal(err)
			}
			_ = data
		}
	}
}

// Compare message sizes
func TestMessageSizeComparison(t *testing.T) {
	codec, err := NewCodec()
	if err != nil {
		t.Fatal(err)
	}

	msg := &Message{
		ID:        uuid.New().String(),
		Type:      TypeChat,
		Timestamp: time.Now(),
		SeqNum:    42,
		Payload: []byte(`{
			"role": "user",
			"content": "Write a function that calculates the fibonacci sequence",
			"files": ["main.py", "utils.py"],
			"metadata": {
				"model": "claude-3-sonnet",
				"temperature": "0.7",
				"max_tokens": 1000
			}
		}`),
	}

	// JSON encoding
	jsonData, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	// Protobuf encoding
	protoData, err := codec.EncodeMessage(msg)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("JSON size: %d bytes", len(jsonData))
	t.Logf("Protobuf size: %d bytes", len(protoData))
	t.Logf("Size reduction: %.1f%%", (1-float64(len(protoData))/float64(len(jsonData)))*100)
}