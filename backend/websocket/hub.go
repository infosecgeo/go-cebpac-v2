package websocket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"cebupac/backend/config"
	"cebupac/backend/logger"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// In production, verify against allowed origins
		origin := r.Header.Get("Origin")
		cfg := config.GetConfig()
		
		// Allow same-origin requests
		if origin == "" || origin == "http://localhost:"+cfg.Server.Port || origin == "https://localhost:"+cfg.Server.Port {
			return true
		}
		
		// TODO: Add production origin whitelist check
		// For now, allow all in development
		return cfg.Server.Environment == "development"
	},
}

type MessageType string

const (
	TypeProgress     MessageType = "progress"
	TypeTaskStart    MessageType = "task_start"
	TypeTaskComplete MessageType = "task_complete"
	TypeTaskError    MessageType = "task_error"
	TypeStatsUpdate  MessageType = "stats_update"
	TypeUserUpdate   MessageType = "user_update"
	TypeLog          MessageType = "log"
	TypeProxyChange  MessageType = "proxy_change"
	TypeRetry        MessageType = "retry"
	TypeHeartbeat    MessageType = "heartbeat"
	TypeKickedOut    MessageType = "kicked_out"
	TypeCreditUpdate MessageType = "credit_update"
	TypeProgressLog  MessageType = "progress_log"
)

type Message struct {
	Type      MessageType            `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

type Client struct {
	ID       string
	UserID   string
	Conn     *websocket.Conn
	Send     chan Message
	Hub      *Hub
	mu       sync.Mutex
}

type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan Message
	mu         sync.RWMutex
}

var hubInstance *Hub
var hubOnce sync.Once

// GetHub returns the singleton Hub instance
func GetHub() *Hub {
	hubOnce.Do(func() {
		hubInstance = &Hub{
			clients:    make(map[*Client]bool),
			register:   make(chan *Client),
			unregister: make(chan *Client),
			broadcast:  make(chan Message, 256),
		}
		go hubInstance.run()
	})
	return hubInstance
}

func (h *Hub) run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			logger.GetLogger().Info("WebSocket client connected", map[string]string{
				"client_id": client.ID,
				"user_id":   client.UserID,
			})

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
			}
			h.mu.Unlock()
			logger.GetLogger().Info("WebSocket client disconnected", map[string]string{
				"client_id": client.ID,
				"user_id":   client.UserID,
			})

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					// Client buffer full, disconnect
					close(client.Send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()

		case <-ticker.C:
			// Send heartbeat to all clients
			h.BroadcastHeartbeat()
		}
	}
}

func (h *Hub) BroadcastMessage(msgType MessageType, data map[string]interface{}) {
	msg := Message{
		Type:      msgType,
		Timestamp: time.Now(),
		Data:      data,
	}
	h.broadcast <- msg
}

func (h *Hub) SendToUser(userID string, msgType MessageType, data map[string]interface{}) {
	msg := Message{
		Type:      msgType,
		Timestamp: time.Now(),
		Data:      data,
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		if client.UserID == userID {
			select {
			case client.Send <- msg:
			default:
				// Skip if buffer full
			}
		}
	}
}

func (h *Hub) BroadcastProgress(currentIndex, total, successCount, failedCount, retryCount, queueLength int, currentTask, currentProxy string) {
	percentage := 0
	if total > 0 {
		percentage = (currentIndex * 100) / total
	}

	h.BroadcastMessage(TypeProgress, map[string]interface{}{
		"current_index":  currentIndex,
		"total":          total,
		"percentage":     percentage,
		"success_count":  successCount,
		"failed_count":   failedCount,
		"retry_count":    retryCount,
		"queue_length":   queueLength,
		"current_task":   currentTask,
		"current_proxy":  currentProxy,
	})
}

func (h *Hub) BroadcastStats(activeUsers, activeSessions int, memoryUsageMB, cpuPercent float64, networkLatencyMs int) {
	h.BroadcastMessage(TypeStatsUpdate, map[string]interface{}{
		"active_users":      activeUsers,
		"active_sessions":   activeSessions,
		"memory_usage_mb":   memoryUsageMB,
		"cpu_percent":       cpuPercent,
		"network_latency_ms": networkLatencyMs,
	})
}

func (h *Hub) BroadcastHeartbeat() {
	h.BroadcastMessage(TypeHeartbeat, map[string]interface{}{
		"timestamp": time.Now().Unix(),
	})
}

// SendKickedOut sends a kick out message to a specific user
func (h *Hub) SendKickedOut(userID string, reason string) {
	h.SendToUser(userID, TypeKickedOut, map[string]interface{}{
		"reason":  reason,
		"message": "Your session has been terminated due to login from another device.",
	})
}

// BroadcastCreditUpdate sends credit update to a user
func (h *Hub) BroadcastCreditUpdate(userID string, credits int) {
	h.SendToUser(userID, TypeCreditUpdate, map[string]interface{}{
		"credits": credits,
	})
}

// SendProgressLog sends a detailed progress log message
func (h *Hub) SendProgressLog(userID string, step string, message string, level string) {
	h.SendToUser(userID, TypeProgressLog, map[string]interface{}{
		"step":    step,
		"message": message,
		"level":   level,
		"time":    time.Now().Format("15:04:05"),
	})
}

func (h *Hub) GetConnectedClients() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ServeWs handles WebSocket requests
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request, userID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.GetLogger().Error("WebSocket upgrade failed", map[string]string{
			"error": err.Error(),
		})
		return
	}

	client := &Client{
		ID:     fmt.Sprintf("%s_%d", userID, time.Now().UnixNano()),
		UserID: userID,
		Conn:   conn,
		Send:   make(chan Message, 256),
		Hub:    hub,
	}

	hub.register <- client

	// Start goroutines for reading and writing
	go client.readPump()
	go client.writePump()
}

func (c *Client) readPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.GetLogger().Error("WebSocket read error", map[string]string{
					"error":     err.Error(),
					"client_id": c.ID,
				})
			}
			break
		}

		// Handle incoming messages if needed
		var msg Message
		if err := json.Unmarshal(message, &msg); err == nil {
			c.handleMessage(msg)
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			data, _ := json.Marshal(message)
			w.Write(data)

			// Add queued messages to current write
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				msg := <-c.Send
				data, _ := json.Marshal(msg)
				w.Write(data)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleMessage(msg Message) {
	// Handle client messages if needed (e.g., ping, preferences, etc.)
	switch msg.Type {
	case TypeHeartbeat:
		// Client heartbeat - do nothing, just reset deadline
	default:
		// Log unknown message types
		logger.GetLogger().Debug("Received WebSocket message", map[string]string{
			"type":      string(msg.Type),
			"client_id": c.ID,
		})
	}
}
