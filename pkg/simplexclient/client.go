// mautrix-simplex - A Matrix-SimpleX puppeting bridge.
// Copyright (C) 2024 Tricked
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package simplexclient

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"
)

// Client is a WebSocket client for the SimpleX Chat API
type Client struct {
	ws      *websocket.Conn
	corrID  atomic.Int64
	mu      sync.Mutex
	pending map[string]chan json.RawMessage

	eventsCh chan Event
	log      zerolog.Logger
	wsURL    string
}

// WireMessage is the JSON structure used on the wire
type WireMessage struct {
	CorrID *string         `json:"corrId"`
	Cmd    string          `json:"cmd,omitempty"`
	Resp   json.RawMessage `json:"resp,omitempty"`
}

// WireEvent is the JSON structure for async events (no corrId)
type WireEvent struct {
	Type string `json:"type"`
}

// New connects to a running simplex-chat instance at wsURL
func New(ctx context.Context, wsURL string, log zerolog.Logger) (*Client, error) {
	ws, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to dial simplex-chat WebSocket at %s: %w", wsURL, err)
	}
	// Increase read limit to 100MB to handle large messages (e.g. images/files in base64)
	ws.SetReadLimit(100 * 1024 * 1024)
	c := &Client{
		ws:       ws,
		pending:  make(map[string]chan json.RawMessage),
		eventsCh: make(chan Event, 64),
		log:      log,
		wsURL:    wsURL,
	}
	go c.readLoop(context.Background())
	return c, nil
}

func (c *Client) Close() error {
	return c.ws.Close(websocket.StatusNormalClosure, "bridge shutting down")
}

// sendRaw sends a raw command string and returns the response bytes
func (c *Client) sendRaw(corrID, cmd string) (json.RawMessage, error) {
	ch := make(chan json.RawMessage, 1)
	c.mu.Lock()
	c.pending[corrID] = ch
	c.mu.Unlock()

	msg := WireMessage{
		CorrID: &corrID,
		Cmd:    cmd,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, corrID)
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to marshal command: %w", err)
	}

	err = c.ws.Write(context.Background(), websocket.MessageText, data)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, corrID)
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to write command: %w", err)
	}

	resp, ok := <-ch
	if !ok {
		return nil, fmt.Errorf("connection closed while waiting for response")
	}
	return resp, nil
}

// sendOneShotCmd opens a fresh WS connection, sends one command, reads the response, and closes.
// Used for file sends where simplex-chat may drop the persistent connection.
func (c *Client) sendOneShotCmd(ctx context.Context, cmd string) (string, json.RawMessage, error) {
	ws, _, err := websocket.Dial(ctx, c.wsURL, nil)
	if err != nil {
		return "", nil, fmt.Errorf("one-shot dial failed: %w", err)
	}
	ws.SetReadLimit(100 * 1024 * 1024)
	defer ws.Close(websocket.StatusNormalClosure, "one-shot done")

	id := c.corrID.Add(1)
	corrID := fmt.Sprintf("%d", id)
	msg := WireMessage{CorrID: &corrID, Cmd: cmd}
	data, err := json.Marshal(msg)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal one-shot command: %w", err)
	}
	if err := ws.Write(ctx, websocket.MessageText, data); err != nil {
		return "", nil, fmt.Errorf("failed to write one-shot command: %w", err)
	}
	// Read responses until we find our corrId response.
	for {
		_, respData, err := ws.Read(ctx)
		if err != nil {
			return "", nil, fmt.Errorf("one-shot read error: %w", err)
		}
		var envelope struct {
			CorrID *string         `json:"corrId"`
			Resp   json.RawMessage `json:"resp"`
		}
		if err := json.Unmarshal(respData, &envelope); err != nil {
			continue
		}
		if envelope.CorrID != nil && *envelope.CorrID == corrID {
			var respType struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(envelope.Resp, &respType); err != nil {
				return "", nil, fmt.Errorf("failed to parse one-shot response type: %w", err)
			}
			return respType.Type, envelope.Resp, nil
		}
	}
}

// sendCmd sends a command and returns the parsed response type + raw bytes
func (c *Client) sendCmd(cmd string) (string, json.RawMessage, error) {
	id := c.corrID.Add(1)
	corrID := fmt.Sprintf("%d", id)
	raw, err := c.sendRaw(corrID, cmd)
	if err != nil {
		return "", nil, err
	}
	var respType struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &respType); err != nil {
		return "", nil, fmt.Errorf("failed to parse response type: %w", err)
	}
	return respType.Type, raw, nil
}

// sendCmdRetryOnce is like sendCmd but on connection loss uses a one-shot WS for the retry.
func (c *Client) sendCmdRetryOnce(ctx context.Context, cmd string) (string, json.RawMessage, error) {
	id := c.corrID.Add(1)
	corrID := fmt.Sprintf("%d", id)
	raw, err := c.sendRaw(corrID, cmd)
	if err == nil {
		// Success on the persistent connection — parse and return.
		var respType struct {
			Type string `json:"type"`
		}
		if jsonErr := json.Unmarshal(raw, &respType); jsonErr != nil {
			return "", nil, fmt.Errorf("failed to parse response type: %w", jsonErr)
		}
		return respType.Type, raw, nil
	}
	// On connection loss, retry via a fresh one-shot connection.
	c.log.Warn().Err(err).Msg("Connection lost during send; retrying with one-shot connection")
	return c.sendOneShotCmd(ctx, cmd)
}

// Events returns the channel for async events
func (c *Client) Events() <-chan Event {
	return c.eventsCh
}

func (c *Client) readLoop(ctx context.Context) {
	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			c.log.Err(err).Msg("WebSocket read error")
			// Signal all pending requests that connection is closed
			c.mu.Lock()
			for _, ch := range c.pending {
				close(ch)
			}
			c.pending = make(map[string]chan json.RawMessage)
			c.mu.Unlock()
			close(c.eventsCh)
			return
		}

		var msg struct {
			CorrID *string         `json:"corrId"`
			Resp   json.RawMessage `json:"resp"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			c.log.Err(err).Msg("Failed to unmarshal WebSocket message")
			continue
		}

		// Always parse the event type for logging
		var typeInfo struct {
			Type string `json:"type"`
		}
		if msg.Resp != nil {
			_ = json.Unmarshal(msg.Resp, &typeInfo)
		}

		if msg.CorrID != nil {
			// This is a response to a command (or a spurious event with a corrId)
			c.mu.Lock()
			ch, ok := c.pending[*msg.CorrID]
			if ok {
				delete(c.pending, *msg.CorrID)
			}
			c.mu.Unlock()
			if ok {
				rawStr := string(msg.Resp)
				if len(rawStr) > 300 {
					rawStr = rawStr[:300]
				}
				c.log.Debug().Str("corr_id", *msg.CorrID).Str("resp_preview", rawStr).Msg("Routing response to pending command")
				ch <- msg.Resp
			} else {
				// No pending command — treat as async event so it's not silently dropped.
				rawStr := string(msg.Resp)
				if len(rawStr) > 300 {
					rawStr = rawStr[:300]
				}
				c.log.Debug().Str("corr_id", *msg.CorrID).Str("event_type", typeInfo.Type).Str("resp_preview", rawStr).Msg("Received event with corrId but no pending command, treating as async event")
				if msg.Resp != nil && typeInfo.Type != "" {
					evt := Event{
						Type: typeInfo.Type,
						Raw:  msg.Resp,
					}
					select {
					case c.eventsCh <- evt:
					default:
						c.log.Warn().Str("event_type", typeInfo.Type).Msg("Event channel full, dropping event")
					}
				}
			}
		} else if msg.Resp != nil {
			// Async event (corrId: null in the response envelope)
			c.log.Debug().Str("event_type", typeInfo.Type).Msg("Received async event")
			if typeInfo.Type == "" {
				c.log.Warn().Str("resp_raw", string(msg.Resp)[:min(200, len(msg.Resp))]).Msg("Async event has no type")
				continue
			}
			evt := Event{
				Type: typeInfo.Type,
				Raw:  msg.Resp,
			}
			select {
			case c.eventsCh <- evt:
			default:
				c.log.Warn().Str("event_type", typeInfo.Type).Msg("Event channel full, dropping event")
			}
		}
	}
}
