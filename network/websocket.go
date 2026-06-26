package network

import (
	"Enserva/objects"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type WebSocketServer struct {
	upgrader    websocket.Upgrader
	clients     map[*websocket.Conn]*WebSocketClient
	world       WorldConfig
	playerRules objects.PlayerRules
	tick        uint64
	mu          sync.Mutex
}

type WebSocketClient struct {
	conn    *websocket.Conn
	player  *objects.Player
	input   InputMessage
	lastSeq uint64
}

func NewWebSocketServer(world WorldConfig) *WebSocketServer {
	playerRules := objects.DefaultPlayerRules(world)

	return &WebSocketServer{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		clients:     map[*websocket.Conn]*WebSocketClient{},
		world:       playerRules.World,
		playerRules: playerRules,
	}
}

func ServeWebSocketDebugClient(world WorldConfig) error {
	return ServeWebSocketDebugClientWithAddress(world, ":8080")
}

func ServeWebSocketDebugClientWithAddress(world WorldConfig, httpAddress string) error {
	webSocketServer := NewWebSocketServer(world)
	go webSocketServer.runTickLoop()

	http.HandleFunc("/ws", webSocketServer.Handle)
	http.Handle("/", serveDebugFiles("debug-frontend-ws"))

	log.Printf("Enserva WebSocket server running on %s", httpAddress)
	if webSocketServer.world.Dimension.Is3D() {
		log.Printf("Game world: %.0fx%.0fx%.0f (%s)", webSocketServer.world.X, webSocketServer.world.Y, webSocketServer.world.Z, webSocketServer.world.Dimension)
	} else {
		log.Printf("Game world: %.0fx%.0f (%s)", webSocketServer.world.X, webSocketServer.world.Y, webSocketServer.world.Dimension)
	}
	log.Printf("Debug client: http://localhost%s/", httpAddress)

	return http.ListenAndServe(httpAddress, nil)
}

func (server *WebSocketServer) Handle(w http.ResponseWriter, r *http.Request) {
	conn, err := server.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	server.mu.Lock()
	client := &WebSocketClient{
		conn:   conn,
		player: objects.SpawnPlayer("ws-"+conn.RemoteAddr().String(), server.world, len(server.clients)),
	}
	server.clients[conn] = client
	server.mu.Unlock()

	defer func() {
		server.mu.Lock()
		delete(server.clients, conn)
		server.mu.Unlock()
	}()

	for {
		var input InputMessage

		if err := conn.ReadJSON(&input); err != nil {
			break
		}

		server.mu.Lock()
		if input.Sequence > client.lastSeq {
			client.input = input
			client.lastSeq = input.Sequence
		}
		server.mu.Unlock()
	}
}

func (server *WebSocketServer) runTickLoop() {
	ticker := time.NewTicker(time.Second / simulationTickRate)
	defer ticker.Stop()

	for range ticker.C {
		tick := server.advance()

		if tick%uint64(simulationTickRate/snapshotRate) == 0 {
			server.broadcastPlayers()
		}
	}
}

func (server *WebSocketServer) advance() uint64 {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.tick++
	for _, client := range server.clients {
		client.player.ApplyInput(client.input.Movement(), (time.Second / simulationTickRate).Seconds(), server.playerRules, server.otherPlayers(client))
	}

	return server.tick
}

func (server *WebSocketServer) broadcastPlayers() {
	snapshots := server.snapshotPlayers()

	for _, snapshot := range snapshots {
		if err := snapshot.conn.WriteJSON(snapshot.message); err != nil {
			log.Println("websocket broadcast error:", err)
			server.removeClient(snapshot.conn)
		}
	}
}

type webSocketSnapshot struct {
	conn    *websocket.Conn
	message SnapshotMessage
}

func (server *WebSocketServer) snapshotPlayers() []webSocketSnapshot {
	server.mu.Lock()
	defer server.mu.Unlock()

	players := map[string]objects.Player{}
	for _, client := range server.clients {
		players[client.player.ID] = client.player.Clone()
	}

	snapshots := make([]webSocketSnapshot, 0, len(server.clients))
	for _, client := range server.clients {
		snapshots = append(snapshots, webSocketSnapshot{
			conn: client.conn,
			message: SnapshotMessage{
				Type:         "snapshot",
				SelfID:       client.player.ID,
				Tick:         server.tick,
				LastSequence: client.lastSeq,
				World:        server.world,
				Players:      players,
			},
		})
	}

	return snapshots
}

func (server *WebSocketServer) removeClient(conn *websocket.Conn) {
	server.mu.Lock()
	delete(server.clients, conn)
	server.mu.Unlock()
}

func (server *WebSocketServer) otherPlayers(client *WebSocketClient) []*objects.Player {
	others := make([]*objects.Player, 0, len(server.clients)-1)

	for _, other := range server.clients {
		if other == client {
			continue
		}

		others = append(others, other.player)
	}

	return others
}
