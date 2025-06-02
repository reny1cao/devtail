package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow connections from any origin for testing
	},
}

func main() {
	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>DevTail Gateway Test</title></head>
<body>
<h1>DevTail Gateway Test</h1>
<div id="messages"></div>
<input type="text" id="messageInput" placeholder="Type a message...">
<button onclick="sendMessage()">Send</button>
<script>
const ws = new WebSocket('ws://localhost:8080/ws');
const messages = document.getElementById('messages');

ws.onmessage = function(event) {
    const div = document.createElement('div');
    div.textContent = 'Received: ' + event.data;
    messages.appendChild(div);
};

function sendMessage() {
    const input = document.getElementById('messageInput');
    ws.send(input.value);
    input.value = '';
}

document.getElementById('messageInput').addEventListener('keypress', function(e) {
    if (e.key === 'Enter') {
        sendMessage();
    }
});
</script>
</body>
</html>`)
	})

	log.Println("Starting DevTail Gateway test server on :8080")
	log.Println("Open http://localhost:8080 in your browser to test")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("New WebSocket connection from %s", r.RemoteAddr)

	// Send welcome message
	welcome := map[string]interface{}{
		"type":    "welcome",
		"message": "Connected to DevTail Gateway",
		"time":    time.Now().Format(time.RFC3339),
	}
	
	if err := conn.WriteJSON(welcome); err != nil {
		log.Printf("Failed to send welcome message: %v", err)
		return
	}

	// Handle messages
	for {
		var msg map[string]interface{}
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		log.Printf("Received message: %v", msg)

		// Echo back with a response
		response := map[string]interface{}{
			"type":     "response",
			"original": msg,
			"echo":     fmt.Sprintf("Gateway received: %v", msg),
			"time":     time.Now().Format(time.RFC3339),
		}

		if err := conn.WriteJSON(response); err != nil {
			log.Printf("Failed to send response: %v", err)
			break
		}
	}

	log.Printf("WebSocket connection closed")
}