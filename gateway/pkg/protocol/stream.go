package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"
)

// MessageReader reads framed messages from a stream
type MessageReader struct {
	reader io.Reader
	codec  *Codec
	mu     sync.Mutex
	buf    []byte
}

// ReadMessage reads the next message from the stream
func (r *MessageReader) ReadMessage() (*Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Read frame header
	header := make([]byte, frameHeaderSize)
	if _, err := io.ReadFull(r.reader, header); err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("read header: %w", err)
	}

	// Parse header
	flags := header[0]
	length := binary.BigEndian.Uint32(header[1:5])

	if length > maxFrameSize {
		return nil, fmt.Errorf("frame too large: %d bytes", length)
	}

	// Read payload
	payload := make([]byte, length)
	if _, err := io.ReadFull(r.reader, payload); err != nil {
		return nil, fmt.Errorf("read payload: %w", err)
	}

	// Reconstruct frame
	frame := make([]byte, frameHeaderSize+length)
	copy(frame[:frameHeaderSize], header)
	copy(frame[frameHeaderSize:], payload)

	// Handle batch messages
	if (flags & flagBatch) != 0 {
		return nil, fmt.Errorf("batch messages not supported in streaming mode")
	}

	// Decode message
	return r.codec.DecodeMessage(frame)
}

// MessageWriter writes framed messages to a stream
type MessageWriter struct {
	writer io.Writer
	codec  *Codec
	mu     sync.Mutex
}

// WriteMessage writes a message to the stream
func (w *MessageWriter) WriteMessage(msg *Message) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Encode message
	data, err := w.codec.EncodeMessage(msg)
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}

	// Write to stream
	if _, err := w.writer.Write(data); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}

	return nil
}

// WriteBatch writes multiple messages as a batch
func (w *MessageWriter) WriteBatch(messages []*Message) error {
	if len(messages) == 0 {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// For single message, just write it normally
	if len(messages) == 1 {
		return w.WriteMessage(messages[0])
	}

	// Encode batch
	data, err := w.codec.EncodeBatch(messages)
	if err != nil {
		return fmt.Errorf("encode batch: %w", err)
	}

	// Write to stream
	if _, err := w.writer.Write(data); err != nil {
		return fmt.Errorf("write batch: %w", err)
	}

	return nil
}

// Flush flushes any buffered data
func (w *MessageWriter) Flush() error {
	if flusher, ok := w.writer.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	return nil
}