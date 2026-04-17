package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"wisemed-labreaders/new/internal/readeragent/command"
	"wisemed-labreaders/new/internal/readeragent/storage"
	"wisemed-labreaders/new/internal/shared/protocol"
)

type WSClient struct {
	WSURL               string
	ReconnectInterval   time.Duration
	HeartbeatInterval   time.Duration
	ReaderID            string
	AnalyzerCode        string
	AnalyzerName        string
	AnalyzerType        string
	LicenseCode         string
	APIKey              string
	APIKeyRef           string
	APIBaseURL          string
	Handler             *command.Handler
	Store               *storage.SQLiteStore
	OnRegistrationState func(setupComplete bool, profile map[string]interface{})
}

func (c *WSClient) Run(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		default:
		}

		conn, err := c.dial()
		if err != nil {
			log.Printf("ws connect error: %v", err)
			time.Sleep(c.ReconnectInterval)
			continue
		}

		log.Printf("connected to control-plane: %s", c.WSURL)
		_ = conn.WriteJSON(protocol.ReaderHelloMessage{
			Type:         protocol.MsgTypeReaderHello,
			ReaderID:     c.ReaderID,
			AnalyzerCode: c.AnalyzerCode,
			AnalyzerName: c.AnalyzerName,
			AnalyzerType: c.AnalyzerType,
			LicenseCode:  c.LicenseCode,
			CreatedAt:    time.Now().UTC(),
		})

		closed := make(chan struct{})
		go c.heartbeatLoop(conn, closed)
		go c.flushResultsLoop(conn, closed)
		c.readLoop(conn)
		close(closed)
		_ = conn.Close()
		log.Printf("ws disconnected, retry in %s", c.ReconnectInterval)
		time.Sleep(c.ReconnectInterval)
	}
}

func (c *WSClient) dial() (*websocket.Conn, error) {
	u, err := url.Parse(c.WSURL)
	if err != nil {
		return nil, err
	}

	header := http.Header{}
	apiKey := strings.TrimSpace(c.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv(c.APIKeyRef))
	}
	if apiKey == "" {
		return nil, errors.New("missing reader api key")
	}
	token, err := c.buildReaderJWT(apiKey)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	header.Set("Authorization", "Bearer "+token)

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), header)
	return conn, err
}

func (c *WSClient) buildReaderJWT(apiKey string) (string, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"sub":           c.ReaderID,
		"role":          "reader",
		"analyzer_code": c.AnalyzerCode,
		"iat":           now.Unix(),
		"exp":           now.Add(5 * time.Minute).Unix(),
		"jti":           fmt.Sprintf("rjwt-%d", now.UnixNano()),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString([]byte(apiKey))
}

func (c *WSClient) heartbeatLoop(conn *websocket.Conn, stop <-chan struct{}) {
	ticker := time.NewTicker(c.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			hb := protocol.HeartbeatMessage{
				Type:     protocol.MsgTypeHeartbeat,
				ReaderID: c.ReaderID,
				Status:   "online",
				Meta: map[string]interface{}{
					"analyzer_code": c.AnalyzerCode,
					"storage":       c.Store.DebugStats(),
				},
				CreatedAt: time.Now().UTC(),
			}
			_ = conn.WriteJSON(hb)
		}
	}
}

func (c *WSClient) flushResultsLoop(conn *websocket.Conn, stop <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			items, err := c.Store.PendingResults(100)
			if err != nil || len(items) == 0 {
				continue
			}
			batch := protocol.ResultBatchMessage{
				Type:      protocol.MsgTypeResultBatch,
				ReaderID:  c.ReaderID,
				Items:     items,
				CreatedAt: time.Now().UTC(),
			}
			_ = conn.WriteJSON(batch)
		}
	}
}

func (c *WSClient) readLoop(conn *websocket.Conn) {
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var env protocol.Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			continue
		}

		switch strings.TrimSpace(env.Type) {
		case protocol.MsgTypeCommand:
			var cmd protocol.CommandMessage
			if err := json.Unmarshal(raw, &cmd); err != nil {
				continue
			}
			success, data, errorText := c.Handler.Handle(cmd.Command, cmd.Args)
			res := protocol.CommandResultMessage{
				Type:          protocol.MsgTypeCommandResult,
				CommandID:     cmd.CommandID,
				CorrelationID: cmd.CorrelationID,
				Success:       success,
				Data:          data,
				Error:         errorText,
				HandledAt:     time.Now().UTC(),
			}
			_ = conn.WriteJSON(res)
		case protocol.MsgTypeResultAck:
			var ack protocol.ResultBatchAckMessage
			if err := json.Unmarshal(raw, &ack); err != nil {
				continue
			}
			_ = c.Store.MarkResultsSent(ack.AcceptedRefs)
		case protocol.MsgTypeRegisterState:
			var reg protocol.RegistrationStateMessage
			if err := json.Unmarshal(raw, &reg); err != nil {
				continue
			}
			_ = c.Store.AppendEvent("registration_state", map[string]interface{}{
				"registered":     reg.Registered,
				"setup_complete": reg.SetupComplete,
				"profile":        reg.Profile,
			})
			if c.OnRegistrationState != nil {
				c.OnRegistrationState(reg.SetupComplete, reg.Profile)
			}
		}
	}
}

func (c *WSClient) ResolveWorklist(sampleID string, patientID string, requestedTags []string) ([]string, error) {
	base := strings.TrimRight(strings.TrimSpace(c.APIBaseURL), "/")
	if base == "" {
		return nil, errors.New("missing api base url")
	}
	apiKey := strings.TrimSpace(c.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv(c.APIKeyRef))
	}
	if apiKey == "" {
		return nil, errors.New("missing reader api key")
	}
	token, err := c.buildReaderJWT(apiKey)
	if err != nil {
		return nil, err
	}
	body := map[string]interface{}{
		"reader_id":  c.ReaderID,
		"sample_id":  sampleID,
		"patient_id": patientID,
		"tags":       requestedTags,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, base+"/api/worklist/resolve", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("worklist resolve failed with status %d", resp.StatusCode)
	}
	var parsed struct {
		ApprovedTags []string `json:"approved_tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	return parsed.ApprovedTags, nil
}
