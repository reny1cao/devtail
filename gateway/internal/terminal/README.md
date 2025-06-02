# Terminal Multiplexing

This package provides PTY-based terminal multiplexing for the DevTail gateway, enabling SSH-like functionality over WebSocket.

## Features

- **Full PTY Support**: Proper terminal emulation with ANSI escape sequences
- **Multiple Sessions**: Support for multiple concurrent terminal sessions
- **Resize Handling**: Dynamic terminal resizing (SIGWINCH)
- **Session Management**: Automatic cleanup of idle sessions
- **Security**: Isolated sessions with configurable timeouts
- **Performance**: Efficient binary streaming with base64 encoding

## Architecture

```
WebSocket Client
      ↓
Unified Handler
      ↓
Terminal Handler
      ↓
Terminal Manager
      ↓
Terminal (PTY)
      ↓
Shell Process
```

## Usage

### Creating a Terminal

```json
{
  "id": "msg-123",
  "type": "terminal_create",
  "payload": {
    "work_dir": "/home/user/project",
    "env": ["CUSTOM_VAR=value"],
    "rows": 24,
    "cols": 80
  }
}
```

Response:
```json
{
  "id": "msg-456",
  "type": "terminal_created",
  "payload": {
    "terminal_id": "term-uuid",
    "success": true
  }
}
```

### Sending Input

```json
{
  "id": "msg-789",
  "type": "terminal_input",
  "payload": {
    "terminal_id": "term-uuid",
    "data": "bHMgLWxhCg=="  // base64("ls -la\n")
  }
}
```

### Receiving Output

The gateway streams terminal output automatically:

```json
{
  "id": "msg-abc",
  "type": "terminal_output",
  "payload": {
    "terminal_id": "term-uuid",
    "data": "dG90YWwgMTYK...",  // base64 encoded
    "stderr": false
  }
}
```

### Resizing Terminal

```json
{
  "id": "msg-def",
  "type": "terminal_resize",
  "payload": {
    "terminal_id": "term-uuid",
    "rows": 40,
    "cols": 120
  }
}
```

### Closing Terminal

```json
{
  "id": "msg-ghi",
  "type": "terminal_close",
  "payload": {
    "terminal_id": "term-uuid"
  }
}
```

## Terminal Manager Configuration

```go
manager := terminal.NewManager(
    terminal.WithMaxSessions(20),              // Max concurrent terminals
    terminal.WithSessionTimeout(30*time.Minute), // Idle timeout
    terminal.WithDefaultShell("/bin/bash"),    // Shell to use
)
```

## Security Considerations

1. **Session Isolation**: Each terminal runs in its own process
2. **Timeouts**: Automatic cleanup of idle sessions
3. **Resource Limits**: Maximum session limits prevent DoS
4. **Environment Control**: Sanitized environment variables

## Testing

Run the terminal test client:
```bash
make test-terminal
```

This provides an interactive terminal session over WebSocket.

## Mobile Client Implementation

For mobile clients, terminal integration requires:

1. **Terminal Emulator**: Use libraries like:
   - iOS: SwiftTerm or iOS-Terminal-Emulator
   - Android: Android-Terminal-Emulator or Termux components

2. **WebSocket Integration**:
```swift
// iOS Example
func sendInput(_ text: String) {
    let data = text.data(using: .utf8)!
    let base64 = data.base64EncodedString()
    
    let msg = TerminalInput(
        terminalId: currentTerminalId,
        data: base64
    )
    
    websocket.send(message: msg)
}

func handleOutput(_ message: TerminalOutput) {
    guard let data = Data(base64Encoded: message.data) else { return }
    guard let text = String(data: data, encoding: .utf8) else { return }
    
    terminalView.write(text)
}
```

3. **Gesture Handling**:
- Pinch to zoom for font size
- Two-finger scroll for history
- Long press for copy/paste
- Swipe for tab switching

## Performance Optimization

1. **Output Buffering**: Terminal output is buffered to reduce message frequency
2. **Binary Protocol**: Use Protocol Buffers for even better performance
3. **Compression**: Large outputs are automatically compressed
4. **Connection Pooling**: Reuse terminal sessions when possible

## Troubleshooting

### Terminal Not Responding
- Check if process is still running
- Verify PTY allocation succeeded
- Check for blocking I/O operations

### Garbled Output
- Ensure TERM environment is set correctly
- Verify client terminal emulator supports ANSI
- Check character encoding (UTF-8)

### Performance Issues
- Reduce terminal scrollback buffer
- Enable output throttling for slow connections
- Use binary WebSocket frames