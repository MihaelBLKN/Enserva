package network

import (
	"Enserva/objects"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type UDPServer struct {
	address       string
	tickInterval  time.Duration
	snapshotEvery uint64
	clients       map[string]*UDPClient
	world         WorldConfig
	playerRules   objects.PlayerRules
	tick          uint64
	mu            sync.Mutex
}

type UDPClient struct {
	addr        *net.UDPAddr
	player      *objects.Player
	input       InputMessage
	lastSeq     uint64
	lastHeardAt time.Time
}

func CreateUDPServer(world WorldConfig) *UDPServer {
	return CreateUDPServerWithAddress(world, ":9000")
}

func CreateUDPServerWithAddress(world WorldConfig, address string) *UDPServer {
	playerRules := objects.DefaultPlayerRules(world)
	if strings.TrimSpace(address) == "" {
		address = ":9000"
	}

	return &UDPServer{
		address:       address,
		tickInterval:  time.Second / simulationTickRate,
		snapshotEvery: simulationTickRate / snapshotRate,
		clients:       map[string]*UDPClient{},
		world:         playerRules.World,
		playerRules:   playerRules,
	}
}

func ServeUDPDebugClient(world WorldConfig) error {
	return ServeUDPDebugClientWithAddresses(world, ":8080", ":9000")
}

func ServeUDPDebugClientWithAddresses(world WorldConfig, httpAddress, udpAddress string) error {
	udpServer := CreateUDPServerWithAddress(world, udpAddress)

	go func() {
		if err := udpServer.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	http.HandleFunc("/ws", handleUDPBridge(udpServer.address))
	http.HandleFunc("/udp-bridge", handleUDPBridge(udpServer.address))
	http.Handle("/", serveDebugFiles(udpDebugDirectory(udpServer.world)))

	log.Printf("Enserva UDP server running on %s", udpServer.address)
	log.Printf("Enserva debug client running on %s", httpAddress)
	if udpServer.world.Dimension.Is3D() {
		log.Printf("Game world: %.0fx%.0fx%.0f (%s)", udpServer.world.X, udpServer.world.Y, udpServer.world.Z, udpServer.world.Dimension)
	} else {
		log.Printf("Game world: %.0fx%.0f (%s)", udpServer.world.X, udpServer.world.Y, udpServer.world.Dimension)
	}
	log.Printf("Debug client: http://localhost%s/", httpAddress)

	return http.ListenAndServe(httpAddress, nil)
}

func udpDebugDirectory(world WorldConfig) string {
	if world.Dimension.Is3D() {
		return "debug-frontend-udp-threeD/dist"
	}

	return "debug-frontend-udp"
}

func (server *UDPServer) ListenAndServe() error {
	addr, err := net.ResolveUDPAddr("udp", server.address)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	go server.runTickLoop(conn)

	buffer := make([]byte, 4096)

	for {
		bytesRead, clientAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Println("udp read error:", err)
			continue
		}

		if err := server.handleMessage(clientAddr, buffer[:bytesRead]); err != nil {
			log.Println("udp message error:", err)
		}
	}
}

func (server *UDPServer) handleMessage(addr *net.UDPAddr, message []byte) error {
	var input InputMessage
	if err := json.Unmarshal(message, &input); err != nil {
		return err
	}

	server.mu.Lock()
	defer server.mu.Unlock()

	client := server.getOrCreateClient(addr)
	client.lastHeardAt = time.Now()

	if input.Sequence <= client.lastSeq {
		return nil
	}

	client.input = input
	client.lastSeq = input.Sequence

	return nil
}

func (server *UDPServer) getOrCreateClient(addr *net.UDPAddr) *UDPClient {
	key := addr.String()
	client, ok := server.clients[key]
	if ok {
		return client
	}

	client = &UDPClient{
		addr:        addr,
		player:      objects.SpawnPlayer("udp-"+key, server.world, len(server.clients)),
		lastHeardAt: time.Now(),
	}
	server.clients[key] = client

	return client
}

func (server *UDPServer) runTickLoop(conn *net.UDPConn) {
	ticker := time.NewTicker(server.tickInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := server.advanceAndMaybeBroadcast(conn); err != nil {
			log.Println("udp tick broadcast error:", err)
		}
	}
}

func (server *UDPServer) advanceAndMaybeBroadcast(conn *net.UDPConn) error {
	snapshots, err := server.advanceAndSnapshot()
	if err != nil || len(snapshots) == 0 {
		return err
	}

	for _, snapshot := range snapshots {
		if _, err := conn.WriteToUDP(snapshot.payload, snapshot.addr); err != nil {
			log.Println("udp broadcast error:", err)
		}
	}

	return nil
}

type udpSnapshot struct {
	addr    *net.UDPAddr
	payload []byte
}

func (server *UDPServer) advanceAndSnapshot() ([]udpSnapshot, error) {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.tick++
	server.removeStaleClients(time.Now())

	for _, client := range server.clients {
		client.player.ApplyInput(client.input.Movement(), server.tickInterval.Seconds(), server.playerRules, server.otherPlayers(client))
	}

	if server.tick%server.snapshotEvery != 0 {
		return nil, nil
	}

	players := map[string]objects.Player{}

	for _, client := range server.clients {
		players[client.player.ID] = client.player.Clone()
	}

	snapshots := make([]udpSnapshot, 0, len(server.clients))
	for _, client := range server.clients {
		payload, err := json.Marshal(SnapshotMessage{
			Type:         "snapshot",
			SelfID:       client.player.ID,
			Tick:         server.tick,
			LastSequence: client.lastSeq,
			World:        server.world,
			Players:      players,
		})
		if err != nil {
			return nil, err
		}

		snapshots = append(snapshots, udpSnapshot{
			addr:    client.addr,
			payload: payload,
		})
	}

	return snapshots, nil
}

func (server *UDPServer) removeStaleClients(now time.Time) {
	timeout := time.Duration(clientTimeout) * time.Second

	for key, client := range server.clients {
		if now.Sub(client.lastHeardAt) > timeout {
			delete(server.clients, key)
		}
	}
}

func (server *UDPServer) otherPlayers(client *UDPClient) []*objects.Player {
	others := make([]*objects.Player, 0, len(server.clients)-1)

	for _, other := range server.clients {
		if other == client {
			continue
		}

		others = append(others, other.player)
	}

	return others
}

var udpBridgeUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func handleUDPBridge(udpAddress string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wsConn, err := udpBridgeUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer wsConn.Close()

		udpAddr, err := net.ResolveUDPAddr("udp", loopbackUDPAddress(udpAddress))
		if err != nil {
			log.Println("udp bridge resolve error:", err)
			return
		}

		udpConn, err := net.DialUDP("udp", nil, udpAddr)
		if err != nil {
			log.Println("udp bridge dial error:", err)
			return
		}
		defer udpConn.Close()

		done := make(chan struct{})
		defer close(done)

		go forwardUDPToWebSocket(done, udpConn, wsConn)

		for {
			_, message, err := wsConn.ReadMessage()
			if err != nil {
				return
			}

			if _, err := udpConn.Write(message); err != nil {
				log.Println("udp bridge write error:", err)
				return
			}
		}
	}
}

func loopbackUDPAddress(address string) string {
	host, port, err := net.SplitHostPort(address)
	if err == nil {
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}

		return net.JoinHostPort(host, port)
	}

	if strings.HasPrefix(address, ":") {
		return "127.0.0.1" + address
	}

	return address
}

func forwardUDPToWebSocket(done <-chan struct{}, udpConn *net.UDPConn, wsConn *websocket.Conn) {
	buffer := make([]byte, 4096)

	for {
		select {
		case <-done:
			return
		default:
		}

		if err := udpConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			log.Println("udp bridge deadline error:", err)
			return
		}

		bytesRead, err := udpConn.Read(buffer)
		if err != nil {
			select {
			case <-done:
				return
			default:
			}

			if errors.Is(err, net.ErrClosed) {
				return
			}

			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}

			log.Println("udp bridge read error:", err)
			return
		}

		if err := wsConn.WriteMessage(websocket.TextMessage, buffer[:bytesRead]); err != nil {
			return
		}
	}
}
