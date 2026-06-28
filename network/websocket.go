package network

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WebSocketServer struct {
	upgrader websocket.Upgrader
	clients  map[*websocket.Conn]*WebSocketClient
	runtime  *Runtime
	mu       sync.Mutex
}

type WebSocketClient struct {
	conn    *websocket.Conn
	id      string
	lastSeq uint64
}

func NewWebSocketServer(runtime *Runtime) *WebSocketServer {
	return &WebSocketServer{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		clients: map[*websocket.Conn]*WebSocketClient{},
		runtime: runtime,
	}
}

func (server *WebSocketServer) Handle(w http.ResponseWriter, r *http.Request) {
	conn, err := server.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	client := &WebSocketClient{
		conn: conn,
		id:   "ws-" + conn.RemoteAddr().String(),
	}

	server.mu.Lock()
	server.clients[conn] = client
	server.mu.Unlock()

	defer server.removeClient(conn)

	for {
		var request RequestMessage
		if err := conn.ReadJSON(&request); err != nil {
			break
		}

		if !server.acceptClientSequence(client, request.Sequence) {
			continue
		}

		if err := server.runtime.HandleRequest(RequestContext{
			Transport:  "ws",
			ClientID:   client.id,
			ReceivedAt: time.Now(),
			Request:    request,
		}); err != nil {
			log.Println("websocket request error:", err)
		}
	}
}

func (server *WebSocketServer) runTickLoop() {
	ticker := time.NewTicker(server.runtime.Config().TickInterval())
	defer ticker.Stop()

	for range ticker.C {
		tick := server.runtime.Advance()
		if tick%server.runtime.Config().SnapshotEvery() == 0 {
			server.broadcastObjects()
		}
	}
}

func (server *WebSocketServer) broadcastObjects() {
	snapshot := server.runtime.Snapshot()

	for _, client := range server.snapshotClients() {
		message := SnapshotMessage{
			Type:         "snapshot",
			ClientID:     client.id,
			Tick:         server.runtime.Tick(),
			LastSequence: client.lastSeq,
			Objects:      snapshot,
		}

		if err := client.conn.WriteJSON(message); err != nil {
			log.Println("websocket broadcast error:", err)
			server.removeClient(client.conn)
		}
	}
}

func (server *WebSocketServer) acceptClientSequence(client *WebSocketClient, sequence uint64) bool {
	if sequence == 0 {
		return true
	}

	server.mu.Lock()
	defer server.mu.Unlock()

	if sequence <= client.lastSeq {
		return false
	}

	client.lastSeq = sequence
	return true
}

func (server *WebSocketServer) snapshotClients() []WebSocketClient {
	server.mu.Lock()
	defer server.mu.Unlock()

	clients := make([]WebSocketClient, 0, len(server.clients))
	for _, client := range server.clients {
		clients = append(clients, *client)
	}

	return clients
}

func (server *WebSocketServer) removeClient(conn *websocket.Conn) {
	server.mu.Lock()
	delete(server.clients, conn)
	server.mu.Unlock()
}
