package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/devtail/gateway/pkg/protocol"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/term"
)

func main() {
	// Parse flags
	var url string
	flag.StringVar(&url, "url", "ws://localhost:8080/ws", "WebSocket URL")
	flag.Parse()

	// Connect to gateway
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer conn.Close()

	// Channels
	done := make(chan struct{})
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// Get terminal size
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width, height = 80, 24
	}

	// Create terminal
	createMsg := protocol.Message{
		ID:        uuid.New().String(),
		Type:      "terminal_create",
		Timestamp: time.Now(),
		Payload: json.RawMessage(fmt.Sprintf(`{
			"work_dir": ".",
			"rows": %d,
			"cols": %d
		}`, height, width)),
	}

	if err := conn.WriteJSON(createMsg); err != nil {
		log.Fatal("write create:", err)
	}

	var terminalID string

	// Read messages
	go func() {
		defer close(done)
		for {
			var msg protocol.Message
			err := conn.ReadJSON(&msg)
			if err != nil {
				log.Println("read:", err)
				return
			}

			switch msg.Type {
			case "terminal_created":
				var resp struct {
					TerminalID string `json:"terminal_id"`
					Success    bool   `json:"success"`
					Error      string `json:"error,omitempty"`
				}
				json.Unmarshal(msg.Payload, &resp)
				
				if !resp.Success {
					log.Fatal("terminal creation failed:", resp.Error)
				}
				
				terminalID = resp.TerminalID
				fmt.Printf("Terminal created: %s\n", terminalID)
				fmt.Println("Type 'exit' to quit")
				fmt.Println("---")

			case "terminal_output":
				var output struct {
					TerminalID string `json:"terminal_id"`
					Data       string `json:"data"`
				}
				json.Unmarshal(msg.Payload, &output)
				
				// Decode and print
				data, _ := base64.StdEncoding.DecodeString(output.Data)
				os.Stdout.Write(data)

			case "terminal_error":
				var errMsg struct {
					Error string `json:"error"`
				}
				json.Unmarshal(msg.Payload, &errMsg)
				fmt.Printf("\nError: %s\n", errMsg.Error)
			}
		}
	}()

	// Wait for terminal to be created
	time.Sleep(500 * time.Millisecond)

	// Set terminal to raw mode for proper input handling
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatal("failed to set raw mode:", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	// Handle window resize
	go func() {
		sigwinch := make(chan os.Signal, 1)
		signal.Notify(sigwinch, syscall.SIGWINCH)
		for range sigwinch {
			width, height, _ := term.GetSize(int(os.Stdout.Fd()))
			if terminalID != "" {
				resizeMsg := protocol.Message{
					ID:        uuid.New().String(),
					Type:      "terminal_resize",
					Timestamp: time.Now(),
					Payload: json.RawMessage(fmt.Sprintf(`{
						"terminal_id": "%s",
						"rows": %d,
						"cols": %d
					}`, terminalID, height, width)),
				}
				conn.WriteJSON(resizeMsg)
			}
		}
	}()

	// Read stdin and send to terminal
	go func() {
		reader := bufio.NewReader(os.Stdin)
		buf := make([]byte, 1024)
		
		for {
			n, err := reader.Read(buf)
			if err != nil {
				return
			}
			
			if n > 0 && terminalID != "" {
				inputMsg := protocol.Message{
					ID:        uuid.New().String(),
					Type:      "terminal_input",
					Timestamp: time.Now(),
					Payload: json.RawMessage(fmt.Sprintf(`{
						"terminal_id": "%s",
						"data": "%s"
					}`, terminalID, base64.StdEncoding.EncodeToString(buf[:n]))),
				}
				
				if err := conn.WriteJSON(inputMsg); err != nil {
					return
				}
			}
		}
	}()

	// Wait for interrupt
	for {
		select {
		case <-done:
			return
		case <-interrupt:
			log.Println("interrupt")

			// Close terminal
			if terminalID != "" {
				closeMsg := protocol.Message{
					ID:        uuid.New().String(),
					Type:      "terminal_close",
					Timestamp: time.Now(),
					Payload:   json.RawMessage(fmt.Sprintf(`{"terminal_id":"%s"}`, terminalID)),
				}
				conn.WriteJSON(closeMsg)
			}

			// Cleanly close connection
			err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
			}

			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}