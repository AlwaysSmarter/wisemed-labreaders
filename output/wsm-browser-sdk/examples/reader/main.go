// Demonstrates authenticated WSS + hello + a safe, read-only command handler.
// This is not the analyzer runtime or a production token refresh implementation.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"wisemed-labreaders/serverlast/wsm-server/internal/server"
)

func main() {
	endpoint := flag.String("url", "", "wss://host/ws")
	tokenPath := flag.String("token-file", "", "File containing a short-lived JWT")
	client := flag.String("client", "reader-client-1", "Must match JWT client_id")
	equipment := flag.String("equipment", "42", "Must match JWT equipment_id")
	reader := flag.String("reader", "reader-1", "Must match JWT reader_id")
	flag.Parse()
	if !strings.HasPrefix(*endpoint, "wss://") {
		log.Fatal("a wss:// URL is required")
	}
	raw, err := os.ReadFile(*tokenPath)
	if err != nil {
		log.Fatal(err)
	}
	header := http.Header{"Authorization": []string{"Bearer " + strings.TrimSpace(string(raw))}}
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	ws, _, err := dialer.Dial(*endpoint, header)
	if err != nil {
		log.Fatal("connection failed (check TLS/JWT/config): ", err)
	}
	defer ws.Close()
	err = ws.WriteJSON(server.Envelope{Type: "hello", Payload: map[string]interface{}{"client_type": "reader", "client_id": *client, "reader_id": *reader, "equipment_id": *equipment}})
	if err != nil {
		log.Fatal(err)
	}
	for {
		var msg server.Envelope
		if err = ws.ReadJSON(&msg); err != nil {
			log.Print("connection ended; obtain a fresh token before reconnecting")
			return
		}
		switch msg.Type {
		case "hello_ack":
			fmt.Println("reader connected")
		case "command":
			sender, _ := msg.Payload["sender_connection_id"].(string)
			if sender == "" || msg.RequestID == "" {
				continue
			}
			payload := map[string]interface{}{"ok": false, "error": map[string]interface{}{"code": "unsupported_command", "message": "Only reader.status is implemented by this example"}}
			if msg.Payload["command"] == "reader.status" {
				payload = map[string]interface{}{"ok": true, "data": map[string]interface{}{"reader_id": *reader, "status": "online", "demo": true}}
			}
			ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err = ws.WriteJSON(server.Envelope{Type: "reply", CorrelationID: msg.RequestID, Target: &server.Target{Mode: "connection", ConnectionID: sender}, Payload: payload}); err != nil {
				return
			}
		}
	}
}
