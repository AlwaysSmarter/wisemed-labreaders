package srv

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

type WSClient struct {
	ServerURL   string
	Serial      string
	Conn        *websocket.Conn
	Reconnect   bool
	SendQueue   chan []byte
	stopChan    chan struct{}
	apiEndpoint string
	apiKey      string
}

func NewWSClient(serial, apiEndpoint, apiKey string) *WSClient {
	return &WSClient{
		Serial:      serial,
		apiEndpoint: apiEndpoint,
		apiKey:      apiKey,
		SendQueue:   make(chan []byte, 100),
		stopChan:    make(chan struct{}),
		Reconnect:   true,
	}
}

func (c *WSClient) fetchServerURL() error {
	reqBody, _ := json.Marshal(map[string]string{
		"serial": c.Serial,
		"apiKey": c.apiKey,
	})
	resp, err := http.Post(c.apiEndpoint, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("WS Ednpoint setter API call failed")
	}
	body, _ := ioutil.ReadAll(resp.Body)

	var result struct {
		URL string `json:"ws_url"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}

	c.ServerURL = result.URL
	return nil
}

func (c *WSClient) ConnectLoop() {
	for {
		err := c.connect()
		if err != nil {
			log.Println("WS connect failed:", c.apiEndpoint, err)
		} else {
			log.Println("WS connected successfully")
		}

		select {
		case <-time.After(5 * time.Second):
			log.Println("Retrying WS connection...", c.apiEndpoint)
		case <-c.stopChan:
			return
		}
	}
}

func (c *WSClient) connect() error {
	if err := c.fetchServerURL(); err != nil {
		return err
	}

	log.Printf("Trying to connect to WS endpoint: %s", c.ServerURL)
	conn, _, err := websocket.DefaultDialer.Dial(c.ServerURL, nil)
	if err != nil {
		return err
	}
	c.Conn = conn

	// Send registration
	reg := map[string]string{"type": "register", "serial": c.Serial}
	regJSON, _ := json.Marshal(reg)
	c.Conn.WriteMessage(websocket.TextMessage, regJSON)

	go c.readLoop()
	go c.writeLoop()

	return nil
}

func (c *WSClient) readLoop() {
	defer c.Conn.Close()
	for {
		_, msg, err := c.Conn.ReadMessage()
		if err != nil {
			log.Println("WS read error:", err)
			break
		}
		log.Printf("WS message: %s", msg)

		// Optionally decode and act on commands
	}
	// restart if disconnected
	go c.ConnectLoop()
}

func (c *WSClient) writeLoop() {
	for {
		select {
		case msg := <-c.SendQueue:
			err := c.Conn.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				log.Println("WS write error:", err)
				return
			}
		case <-c.stopChan:
			return
		}
	}
}

func (c *WSClient) Send(msg []byte) {
	select {
	case c.SendQueue <- msg:
	default:
		log.Println("SendQueue full, dropping message")
	}
}

func (c *WSClient) Close() {
	close(c.stopChan)
	if c.Conn != nil {
		c.Conn.Close()
	}
}
