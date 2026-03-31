package server

import (
	"context"
	"log/slog"
	"time"

	"nhooyr.io/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 45 * time.Second
	maxMessageSize = 64 * 1024
	sendBufSize    = 256
)

// Client represents a single WebSocket connection to the hub.
type Client struct {
	hub         *Hub
	conn        *websocket.Conn
	send        chan []byte
	publicKey   []byte
	displayName string
	logger      *slog.Logger
}

func NewClient(hub *Hub, conn *websocket.Conn, publicKey []byte, displayName string, logger *slog.Logger) *Client {
	return &Client{
		hub:         hub,
		conn:        conn,
		send:        make(chan []byte, sendBufSize),
		publicKey:   publicKey,
		displayName: displayName,
		logger:      logger,
	}
}

// ReadPump reads messages from the WebSocket and forwards them to the hub.
func (c *Client) ReadPump(ctx context.Context) {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close(websocket.StatusNormalClosure, "")
	}()

	c.conn.SetReadLimit(maxMessageSize)

	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				c.logger.Debug("client closed connection normally")
			} else {
				c.logger.Debug("read error", "error", err)
			}
			return
		}

		c.hub.incoming <- clientMessage{client: c, data: data}
	}
}

// WritePump writes messages from the hub to the WebSocket.
func (c *Client) WritePump(ctx context.Context) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close(websocket.StatusNormalClosure, "")
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, writeWait)
			err := c.conn.Write(writeCtx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				c.logger.Debug("write error", "error", err)
				return
			}

		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, writeWait)
			err := c.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				c.logger.Debug("ping failed", "error", err)
				return
			}

		case <-ctx.Done():
			return
		}
	}
}
