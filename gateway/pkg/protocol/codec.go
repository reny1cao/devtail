package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sync"

	"github.com/klauspost/compress/zstd"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/devtail/gateway/pkg/protocol/pb"
)

const (
	// Frame header format:
	// [1 byte flags][4 bytes length][payload]
	frameHeaderSize = 5
	
	// Flags
	flagCompressed = 0x01
	flagBatch      = 0x02
	
	// Limits
	maxFrameSize = 1 << 20 // 1MB
	minCompressSize = 1024 // Don't compress small messages
)

// Codec handles Protocol Buffer encoding/decoding with compression
type Codec struct {
	encoder *zstd.Encoder
	decoder *zstd.Decoder
	pool    sync.Pool
}

// NewCodec creates a new Protocol Buffer codec
func NewCodec() (*Codec, error) {
	encoder, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.SpeedFastest),
		zstd.WithEncoderConcurrency(1),
	)
	if err != nil {
		return nil, fmt.Errorf("create zstd encoder: %w", err)
	}

	decoder, err := zstd.NewReader(nil,
		zstd.WithDecoderConcurrency(1),
		zstd.WithDecoderMaxMemory(32<<20), // 32MB max
	)
	if err != nil {
		return nil, fmt.Errorf("create zstd decoder: %w", err)
	}

	return &Codec{
		encoder: encoder,
		decoder: decoder,
		pool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}, nil
}

// EncodeMessage encodes a message to wire format
func (c *Codec) EncodeMessage(msg *Message) ([]byte, error) {
	// Convert to protobuf
	pbMsg, err := c.messageToProto(msg)
	if err != nil {
		return nil, fmt.Errorf("convert to proto: %w", err)
	}

	// Marshal to bytes
	data, err := proto.Marshal(pbMsg)
	if err != nil {
		return nil, fmt.Errorf("marshal proto: %w", err)
	}

	// Frame the message
	return c.frameMessage(data)
}

// DecodeMessage decodes a message from wire format
func (c *Codec) DecodeMessage(data []byte) (*Message, error) {
	// Unframe the message
	payload, compressed, err := c.unframeMessage(data)
	if err != nil {
		return nil, fmt.Errorf("unframe message: %w", err)
	}

	// Decompress if needed
	if compressed {
		decompressed, err := c.decompress(payload)
		if err != nil {
			return nil, fmt.Errorf("decompress: %w", err)
		}
		payload = decompressed
	}

	// Unmarshal protobuf
	var pbMsg pb.Message
	if err := proto.Unmarshal(payload, &pbMsg); err != nil {
		return nil, fmt.Errorf("unmarshal proto: %w", err)
	}

	// Convert to domain message
	return c.protoToMessage(&pbMsg)
}

// EncodeBatch encodes multiple messages into a single frame
func (c *Codec) EncodeBatch(messages []*Message) ([]byte, error) {
	batch := &pb.BatchMessage{
		Messages: make([]*pb.Message, len(messages)),
	}

	for i, msg := range messages {
		pbMsg, err := c.messageToProto(msg)
		if err != nil {
			return nil, fmt.Errorf("convert message %d: %w", i, err)
		}
		batch.Messages[i] = pbMsg
	}

	data, err := proto.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("marshal batch: %w", err)
	}

	// Always compress batches
	compressed, err := c.compress(data)
	if err != nil {
		return nil, fmt.Errorf("compress batch: %w", err)
	}

	return c.frameMessageWithFlags(compressed, flagBatch|flagCompressed)
}

// Reader creates a message reader for streaming
func (c *Codec) Reader(r io.Reader) *MessageReader {
	return &MessageReader{
		reader: r,
		codec:  c,
	}
}

// Writer creates a message writer for streaming
func (c *Codec) Writer(w io.Writer) *MessageWriter {
	return &MessageWriter{
		writer: w,
		codec:  c,
	}
}

// Internal methods

func (c *Codec) frameMessage(data []byte) ([]byte, error) {
	flags := byte(0)
	payload := data

	// Compress if beneficial
	if len(data) > minCompressSize {
		compressed, err := c.compress(data)
		if err != nil {
			return nil, err
		}
		if len(compressed) < len(data)*9/10 { // 10% savings
			flags |= flagCompressed
			payload = compressed
		}
	}

	return c.frameMessageWithFlags(payload, flags)
}

func (c *Codec) frameMessageWithFlags(payload []byte, flags byte) ([]byte, error) {
	if len(payload) > maxFrameSize {
		return nil, fmt.Errorf("message too large: %d bytes", len(payload))
	}

	buf := c.pool.Get().(*bytes.Buffer)
	defer c.pool.Put(buf)
	buf.Reset()

	// Write header
	buf.WriteByte(flags)
	binary.Write(buf, binary.BigEndian, uint32(len(payload)))
	buf.Write(payload)

	// Copy to new slice
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	return result, nil
}

func (c *Codec) unframeMessage(data []byte) (payload []byte, compressed bool, err error) {
	if len(data) < frameHeaderSize {
		return nil, false, fmt.Errorf("frame too small: %d bytes", len(data))
	}

	flags := data[0]
	length := binary.BigEndian.Uint32(data[1:5])

	if length > maxFrameSize {
		return nil, false, fmt.Errorf("frame too large: %d bytes", length)
	}

	if len(data) != int(frameHeaderSize+length) {
		return nil, false, fmt.Errorf("frame size mismatch: expected %d, got %d", 
			frameHeaderSize+length, len(data))
	}

	payload = data[frameHeaderSize:]
	compressed = (flags & flagCompressed) != 0
	return payload, compressed, nil
}

func (c *Codec) compress(data []byte) ([]byte, error) {
	return c.encoder.EncodeAll(data, nil), nil
}

func (c *Codec) decompress(data []byte) ([]byte, error) {
	return c.decoder.DecodeAll(data, nil)
}

func (c *Codec) messageToProto(msg *Message) (*pb.Message, error) {
	pbMsg := &pb.Message{
		Id:           msg.ID,
		Type:         c.messageTypeToProto(msg.Type),
		Timestamp:    timestamppb.New(msg.Timestamp),
		SeqNum:       msg.SeqNum,
		RequiresAck:  msg.RequiresAck,
		RetryCount:   int32(msg.RetryCount),
		CorrelationId: msg.CorrelationID,
	}

	// Convert payload based on type
	if msg.Payload != nil {
		any, err := c.payloadToAny(msg.Type, msg.Payload)
		if err != nil {
			return nil, fmt.Errorf("convert payload: %w", err)
		}
		pbMsg.Payload = any
	}

	return pbMsg, nil
}

func (c *Codec) protoToMessage(pbMsg *pb.Message) (*Message, error) {
	msg := &Message{
		ID:            pbMsg.Id,
		Type:          c.protoToMessageType(pbMsg.Type),
		Timestamp:     pbMsg.Timestamp.AsTime(),
		SeqNum:        pbMsg.SeqNum,
		RequiresAck:   pbMsg.RequiresAck,
		RetryCount:    int(pbMsg.RetryCount),
		CorrelationID: pbMsg.CorrelationId,
	}

	// Convert payload based on type
	if pbMsg.Payload != nil {
		payload, err := c.anyToPayload(msg.Type, pbMsg.Payload)
		if err != nil {
			return nil, fmt.Errorf("convert payload: %w", err)
		}
		msg.Payload = payload
	}

	return msg, nil
}

func (c *Codec) messageTypeToProto(t MessageType) pb.MessageType {
	switch t {
	case TypeChat:
		return pb.MessageType_MESSAGE_TYPE_CHAT
	case TypeChatReply:
		return pb.MessageType_MESSAGE_TYPE_CHAT_REPLY
	case TypeChatStream:
		return pb.MessageType_MESSAGE_TYPE_CHAT_STREAM
	case TypeChatError:
		return pb.MessageType_MESSAGE_TYPE_CHAT_ERROR
	case TypePing:
		return pb.MessageType_MESSAGE_TYPE_PING
	case TypePong:
		return pb.MessageType_MESSAGE_TYPE_PONG
	case TypeAck:
		return pb.MessageType_MESSAGE_TYPE_ACK
	case TypeReconnect:
		return pb.MessageType_MESSAGE_TYPE_RECONNECT
	default:
		return pb.MessageType_MESSAGE_TYPE_UNKNOWN
	}
}

func (c *Codec) protoToMessageType(t pb.MessageType) MessageType {
	switch t {
	case pb.MessageType_MESSAGE_TYPE_CHAT:
		return TypeChat
	case pb.MessageType_MESSAGE_TYPE_CHAT_REPLY:
		return TypeChatReply
	case pb.MessageType_MESSAGE_TYPE_CHAT_STREAM:
		return TypeChatStream
	case pb.MessageType_MESSAGE_TYPE_CHAT_ERROR:
		return TypeChatError
	case pb.MessageType_MESSAGE_TYPE_PING:
		return TypePing
	case pb.MessageType_MESSAGE_TYPE_PONG:
		return TypePong
	case pb.MessageType_MESSAGE_TYPE_ACK:
		return TypeAck
	case pb.MessageType_MESSAGE_TYPE_RECONNECT:
		return TypeReconnect
	default:
		return MessageType("unknown")
	}
}

func (c *Codec) payloadToAny(msgType MessageType, payload []byte) (*anypb.Any, error) {
	// This would convert JSON payloads to proper protobuf types
	// For now, we'll store as raw bytes
	return &anypb.Any{
		TypeUrl: string(msgType),
		Value:   payload,
	}, nil
}

func (c *Codec) anyToPayload(msgType MessageType, any *anypb.Any) ([]byte, error) {
	// This would convert protobuf types back to JSON
	// For now, we'll return raw bytes
	return any.Value, nil
}