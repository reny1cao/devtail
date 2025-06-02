# WebSocket Library Migration Guide (2025)

## Current Status: Gorilla WebSocket in Archive Mode

The `github.com/gorilla/websocket` library has been in **archive mode** since December 2022, meaning:
- ❌ No new features or improvements
- ❌ No security updates  
- ❌ Read-only repository
- ⚠️ Potential future security liability

## Recommended Alternative: Coder WebSocket

**Library**: `github.com/coder/websocket` (took over `nhooyr.io/websocket` in October 2024)

### Why Coder WebSocket?

✅ **Actively maintained** by Coder (2024-2025)  
✅ **Official recommendation** by Go authors  
✅ **Performance claims** - potentially faster than Gorilla  
✅ **Modern Go idioms** - better context.Context support  
✅ **WebAssembly support** - for future mobile cross-platform  
✅ **Used by major projects** - Traefik, Vault, Cloudflare  

## Migration Assessment for DevTail

### Current Gorilla Usage in DevTail:
```go
// gateway/internal/websocket/handler.go
import "github.com/gorilla/websocket"

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

conn, err := upgrader.Upgrade(w, r, nil)
```

### Coder WebSocket Equivalent:
```go
// New approach with coder/websocket
import "github.com/coder/websocket"

conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
    InsecureSkipVerify: true, // For development
})
```

## Migration Strategy

### Phase 1: Add Coder WebSocket (No Breaking Changes)
```bash
# Add new dependency alongside existing
cd gateway
go get github.com/coder/websocket@latest
```

### Phase 2: Create New Handler (Side-by-Side)
```go
// gateway/internal/websocket/coder_handler.go
package websocket

import (
    "context"
    "net/http"
    
    "github.com/coder/websocket"
    "github.com/coder/websocket/wsjson"
)

type CoderHandler struct {
    // Same interface as existing handler
}

func (h *CoderHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
    conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
        InsecureSkipVerify: true, // Configure properly for production
    })
    if err != nil {
        return
    }
    defer conn.Close(websocket.StatusInternalError, "handler error")
    
    ctx := r.Context()
    
    // Handle messages with modern context support
    for {
        var msg map[string]interface{}
        err := wsjson.Read(ctx, conn, &msg)
        if err != nil {
            break
        }
        
        // Process message...
        
        err = wsjson.Write(ctx, conn, response)
        if err != nil {
            break
        }
    }
}
```

### Phase 3: A/B Test Both Implementations
```go
// gateway/cmd/gateway/main.go
func main() {
    // Flag to choose WebSocket implementation
    useCoderWS := flag.Bool("coder-ws", false, "Use Coder WebSocket instead of Gorilla")
    
    if *useCoderWS {
        handler = &websocket.CoderHandler{}
    } else {
        handler = &websocket.GorillaHandler{} // existing
    }
}
```

### Phase 4: Performance Testing
```bash
# Test with Gorilla
./bin/gateway --log-level debug

# Test with Coder 
./bin/gateway --coder-ws --log-level debug

# Compare:
# - Latency
# - Memory usage  
# - CPU usage
# - Mobile battery impact
```

## API Differences Summary

| Feature | Gorilla WebSocket | Coder WebSocket |
|---------|-------------------|-----------------|
| **Upgrade** | `upgrader.Upgrade()` | `websocket.Accept()` |
| **Context** | Manual handling | Native `context.Context` |
| **JSON** | Manual marshal/unmarshal | `wsjson.Read/Write()` |
| **Concurrent Writes** | Need mutex | Supported out of box |
| **Compression** | Manual setup | Built-in support |
| **WebAssembly** | No support | Native support |

## Migration Benefits for DevTail

### Performance Improvements
- **Lower CPU usage** for mobile battery life
- **Better memory allocation** patterns
- **Concurrent write support** without manual locking

### Developer Experience  
- **Context-aware** operations (timeouts, cancellation)
- **Cleaner APIs** following modern Go patterns
- **Better error handling** with structured errors

### Future-Proofing
- **Active maintenance** ensures security updates
- **WebAssembly support** for React Native bridges
- **HTTP/2 compatibility** for next-gen protocols

## Timeline Recommendation

### Immediate (This Sprint)
- ✅ Keep using Gorilla (stable, works)
- ✅ Add Coder as dependency
- ✅ Create proof-of-concept handler

### Short Term (Next Month)
- 🔄 Implement side-by-side A/B testing
- 🔄 Performance benchmarks
- 🔄 Load testing comparison

### Medium Term (Next Quarter)  
- 🔄 Full migration if performance is better
- 🔄 Remove Gorilla dependency
- 🔄 Update documentation

## Risk Assessment

### Low Risk Migration
- **Gorilla still works** - no immediate urgency
- **Similar APIs** - straightforward porting
- **Well-tested library** - used by major projects

### Potential Issues
- **Learning curve** for new APIs
- **Different error handling** patterns
- **Testing overhead** during transition

## Decision: Recommended Approach

For DevTail, I recommend:

1. **Continue with Gorilla** for initial launch (stable, proven)
2. **Add Coder as dependency** and create experimental handler
3. **A/B test performance** with mobile clients
4. **Migrate if performance benefits** are measurable

This approach minimizes risk while positioning for future improvements.