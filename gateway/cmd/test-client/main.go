package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/devtail/gateway/pkg/protocol"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

func main() {
	var url string
	flag.StringVar(&url, "url", "ws://localhost:8080/ws", "WebSocket URL")
	flag.Parse()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	log.Printf("Connecting to %s", url)

	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			var msg protocol.Message
			err := c.ReadJSON(&msg)
			if err != nil {
				log.Println("read:", err)
				return
			}
			
			switch msg.Type {
			case protocol.TypeChatStream:
				var reply protocol.ChatReply
				json.Unmarshal(msg.Payload, &reply)
				fmt.Print(reply.Content)
				if reply.Finished {
					fmt.Println()
				}
			case protocol.TypeChatError:
				var chatErr protocol.ChatError
				json.Unmarshal(msg.Payload, &chatErr)
				fmt.Printf("\nError: %s\n", chatErr.Error)
			case protocol.TypePing:
				pong := protocol.Message{
					ID:        uuid.New().String(),
					Type:      protocol.TypePong,
					Timestamp: time.Now(),
				}
				c.WriteJSON(pong)
			}
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	chatPayload, _ := json.Marshal(protocol.ChatMessage{
		Role:    "user",
		Content: "Hello! Can you see this message?",
	})

	msg := protocol.Message{
		ID:        uuid.New().String(),
		Type:      protocol.TypeChat,
		Timestamp: time.Now(),
		Payload:   chatPayload,
	}

	err = c.WriteJSON(msg)
	if err != nil {
		log.Println("write:", err)
		return
	}

	for {
		select {
		case <-done:
			return
		case <-interrupt:
			log.Println("interrupt")

			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}