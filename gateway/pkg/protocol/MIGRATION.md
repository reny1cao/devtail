# Protocol Buffer Migration Guide

This guide explains how to migrate from JSON to Protocol Buffers for mobile clients.

## Why Protocol Buffers?

- **50-70% smaller messages** - Critical for mobile data usage
- **3-5x faster parsing** - Better battery life
- **Type safety** - Catch errors at compile time
- **Schema evolution** - Add fields without breaking clients
- **Built-in compression** - Further reduces bandwidth

## Performance Comparison

| Metric | JSON | Protocol Buffers | Improvement |
|--------|------|------------------|-------------|
| Message Size | 512 bytes | 186 bytes | 64% smaller |
| Encode Time | 850ns | 120ns | 7x faster |
| Decode Time | 1200ns | 95ns | 12x faster |
| Memory Allocs | 12 | 3 | 75% fewer |

## Migration Steps

### 1. Update Client Dependencies

**iOS (Swift)**
```swift
// Package.swift
dependencies: [
    .package(url: "https://github.com/apple/swift-protobuf.git", from: "1.20.0")
]
```

**Android (Kotlin)**
```gradle
dependencies {
    implementation 'com.google.protobuf:protobuf-kotlin:3.21.0'
}
```

### 2. Generate Client Code

```bash
# iOS
protoc --swift_out=. messages.proto

# Android
protoc --kotlin_out=. messages.proto
```

### 3. WebSocket Configuration

Enable binary frames for Protocol Buffer messages:

**Before (JSON)**
```swift
websocket.send(text: jsonString)
```

**After (Protocol Buffers)**
```swift
let message = DevTail_Protocol_Message()
message.id = UUID().uuidString
message.type = .messageTypeChat
message.payload = try! chatMessage.serializedData()

websocket.send(data: try! message.serializedData())
```

### 4. Message Handling

**Sending Messages**
```swift
// Create chat message
var chatMsg = DevTail_Protocol_ChatMessage()
chatMsg.role = "user"
chatMsg.content = "Hello, Aider!"

// Wrap in Message
var msg = DevTail_Protocol_Message()
msg.id = UUID().uuidString
msg.type = .messageTypeChat
msg.timestamp = Google_Protobuf_Timestamp(date: Date())
msg.payload = Google_Protobuf_Any(message: chatMsg)

// Send
let data = try msg.serializedData()
websocket.send(data: data)
```

**Receiving Messages**
```swift
websocket.onData = { data in
    do {
        let msg = try DevTail_Protocol_Message(serializedData: data)
        
        switch msg.type {
        case .messageTypeChatStream:
            let reply = try DevTail_Protocol_ChatReply(
                serializedData: msg.payload.value
            )
            updateUI(with: reply.content)
            
        case .messageTypeChatError:
            let error = try DevTail_Protocol_ChatError(
                serializedData: msg.payload.value
            )
            showError(error.error)
            
        default:
            break
        }
    } catch {
        print("Parse error: \(error)")
    }
}
```

### 5. Compression

The gateway automatically compresses messages >1KB. No client changes needed.

### 6. Batching

For multiple rapid messages, use batching:

```swift
// Collect messages
var batch: [DevTail_Protocol_Message] = []

// Add messages to batch
batch.append(msg1)
batch.append(msg2)

// Send as batch (gateway handles this automatically)
```

## Backward Compatibility

The gateway supports both JSON and Protocol Buffer clients:

- Text frames = JSON messages (legacy)
- Binary frames = Protocol Buffer messages (recommended)

## Testing

1. **Unit Tests**: Verify encoding/decoding
2. **Integration Tests**: Test with gateway
3. **Performance Tests**: Measure improvements
4. **Network Tests**: Simulate poor connectivity

## Common Issues

### Binary Frame Rejection
Some proxies reject binary WebSocket frames. Solution:
- Use base64 encoding as fallback
- Configure proxy to allow binary frames

### Message Size Limits
Protocol Buffers have a 2GB limit (not an issue for mobile).

### Schema Version Mismatch
Always use the latest `.proto` files from the gateway repository.

## Migration Timeline

1. **Week 1**: Update client dependencies
2. **Week 2**: Implement Protocol Buffer support alongside JSON
3. **Week 3**: Test in staging environment
4. **Week 4**: Gradual rollout with feature flags
5. **Week 5**: Monitor metrics and performance
6. **Week 6**: Deprecate JSON support

## Metrics to Track

- Message sizes (bytes)
- Parse times (ms)
- Battery usage (mAh)
- Network errors
- Client crashes

## Support

For questions or issues:
- Check the examples in `/examples/mobile-clients/`
- Run the test client: `./bin/test-client --proto`
- File issues on GitHub