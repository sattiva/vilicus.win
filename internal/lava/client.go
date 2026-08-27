package lava


import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const apiBase = "/v4"

var ErrNoSession = errors.New("lavalink: no websocket session yet")

type Client struct {
	host     string
	port     int
	password string
	secure   bool

	UserID   string
	ClientNm string

	http *http.Client

	mu        sync.RWMutex
	sessionID string
	conn      *websocket.Conn

	OnEvent func(msg ServerMessage)
}

func NewClient(host string, port int, password string, secure bool, userID, clientName string) *Client {
	return &Client{
		host:     host,
		port:     port,
		password: password,
		secure:   secure,
		UserID:   userID,
		ClientNm: clientName,
		http:     &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) base() string {
	scheme := "http"
	if c.secure {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, c.host, c.port)
}

func (c *Client) SessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionID
}

func (c *Client) Connected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil
}

func (c *Client) Close() {
	c.mu.Lock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()
}

func (c *Client) Dial(ctx context.Context) error {
	scheme := "ws"
	if c.secure {
		scheme = "wss"
	}
	wsURL := url.URL{Scheme: scheme, Host: fmt.Sprintf("%s:%d", c.host, c.port), Path: apiBase + "/websocket"}

	hdr := http.Header{}
	hdr.Set("Authorization", c.password)
	hdr.Set("User-Id", c.UserID)
	hdr.Set("Client-Name", c.ClientNm)
	hdr.Set("Session-Id", randomID())

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL.String(), hdr)
	if err != nil {
		return fmt.Errorf("lavalink dial: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.sessionID = ""
	c.mu.Unlock()

	go c.readLoop(conn)
	return nil
}

func (c *Client) readLoop(conn *websocket.Conn) {
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			if c.conn == conn {
				c.conn = nil
			}
			c.mu.Unlock()
			if c.OnEvent != nil {
				c.OnEvent(ServerMessage{Op: "disconnect"})
			}
			return
		}
		var msg ServerMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		if msg.Op == "ready" && msg.SessionID != "" {
			c.mu.Lock()
			c.sessionID = msg.SessionID
			c.mu.Unlock()
		}
		if c.OnEvent != nil {
			c.OnEvent(msg)
		}
	}
}

func (c *Client) send(v any) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return ErrNoSession
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != conn {
		return ErrNoSession
	}
	return conn.WriteJSON(v)
}

func (c *Client) SendVoiceUpdate(guildID, discordSessionID, token, endpoint string) error {
	return c.send(map[string]any{
		"op":        "voiceUpdate",
		"guildId":   guildID,
		"sessionId": discordSessionID,
		"event": map[string]any{
			"token":    token,
			"endpoint": endpoint,
			"guildId":  guildID,
		},
	})
}


func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.password)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<22))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lavalink %s: status %d: %s", path, resp.StatusCode, truncateBody(body))
	}
	return json.Unmarshal(body, out)
}

func (c *Client) LoadTracks(ctx context.Context, identifier string) (*LoadResult, error) {
	var raw struct {
		LoadType  string          `json:"loadType"`
		Data      json.RawMessage `json:"data"`
		Exception *struct {
			Message  string `json:"message"`
			Severity string `json:"severity"`
		} `json:"exception,omitempty"`
	}
	path := apiBase + "/loadtracks?identifier=" + url.QueryEscape(identifier)
	if err := c.getJSON(ctx, path, &raw); err != nil {
		return nil, err
	}
	if raw.LoadType == "LOAD_FAILED" {
		msg := "unknown lavalink exception"
		if raw.Exception != nil {
			msg = raw.Exception.Message
		}
		return nil, fmt.Errorf("lavalink load failed: %s", msg)
	}
	return ParseLoad(raw.LoadType, raw.Data)
}

func (c *Client) UpdatePlayer(ctx context.Context, guildID string, patch PlayerPatch) error {
	sid := c.SessionID()
	if sid == "" {
		return ErrNoSession
	}
	buf, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
		c.base()+fmt.Sprintf("%s/sessions/%s/players/%s?noReplace=false", apiBase, sid, guildID),
		bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.password)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("lavalink update player: status %d: %s", resp.StatusCode, truncateBody(body))
	}
	return nil
}

func (c *Client) DestroyPlayer(ctx context.Context, guildID string) error {
	sid := c.SessionID()
	if sid == "" {
		return ErrNoSession
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		c.base()+fmt.Sprintf("%s/sessions/%s/players/%s", apiBase, sid, guildID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.password)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("lavalink destroy player: status %d", resp.StatusCode)
	}
	return nil
}

func truncateBody(b []byte) string {
	if len(b) > 200 {
		return string(b[:200])
	}
	return string(b)
}

