package network

import (
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
	address string
	clients map[string]*UDPClient
	runtime *Runtime
	mu      sync.Mutex
}

type UDPClient struct {
	addr        *net.UDPAddr
	id          string
	lastSeq     uint64
	lastHeardAt time.Time
}

func NewUDPServer(runtime *Runtime) *UDPServer {
	return &UDPServer{
		address: runtime.Config().UDPAddress,
		clients: map[string]*UDPClient{},
		runtime: runtime,
	}
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

	buffer := make([]byte, 65535)
	for {
		bytesRead, clientAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Println("udp read error:", err)
			continue
		}

		if err := server.handleMessage(clientAddr, buffer[:bytesRead]); err != nil {
			log.Println("udp request error:", err)
		}
	}
}

func (server *UDPServer) handleMessage(addr *net.UDPAddr, message []byte) error {
	var request RequestMessage
	if err := json.Unmarshal(message, &request); err != nil {
		return err
	}

	client, accepted := server.acceptClientRequest(addr, request.Sequence)
	if !accepted {
		return nil
	}

	return server.runtime.HandleRequest(RequestContext{
		Transport:  "udp",
		ClientID:   client.id,
		ReceivedAt: time.Now(),
		Request:    request,
	})
}

func (server *UDPServer) acceptClientRequest(addr *net.UDPAddr, sequence uint64) (*UDPClient, bool) {
	server.mu.Lock()
	defer server.mu.Unlock()

	client := server.getOrCreateClientLocked(addr)
	client.lastHeardAt = time.Now()

	if sequence == 0 {
		return client, true
	}
	if sequence <= client.lastSeq {
		return client, false
	}

	client.lastSeq = sequence
	return client, true
}

func (server *UDPServer) getOrCreateClientLocked(addr *net.UDPAddr) *UDPClient {
	key := addr.String()
	client, ok := server.clients[key]
	if ok {
		return client
	}

	client = &UDPClient{
		addr:        addr,
		id:          "udp-" + key,
		lastHeardAt: time.Now(),
	}
	server.clients[key] = client

	return client
}

func (server *UDPServer) runTickLoop(conn *net.UDPConn) {
	ticker := time.NewTicker(server.runtime.Config().TickInterval())
	defer ticker.Stop()

	for range ticker.C {
		if err := server.advanceAndMaybeBroadcast(conn); err != nil {
			log.Println("udp snapshot error:", err)
		}
	}
}

func (server *UDPServer) advanceAndMaybeBroadcast(conn *net.UDPConn) error {
	tick := server.runtime.Advance()
	server.removeStaleClients(time.Now())

	if tick%server.runtime.Config().SnapshotEvery() != 0 {
		return nil
	}

	snapshots, err := server.snapshots()
	if err != nil {
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

func (server *UDPServer) snapshots() ([]udpSnapshot, error) {
	objects := server.runtime.Snapshot()
	tick := server.runtime.Tick()
	clients := server.snapshotClients()

	snapshots := make([]udpSnapshot, 0, len(clients))
	for _, client := range clients {
		payload, err := json.Marshal(SnapshotMessage{
			Type:         "snapshot",
			ClientID:     client.id,
			Tick:         tick,
			LastSequence: client.lastSeq,
			Objects:      objects,
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
	server.mu.Lock()
	defer server.mu.Unlock()

	timeout := server.runtime.Config().ClientTimeout
	for key, client := range server.clients {
		if now.Sub(client.lastHeardAt) > timeout {
			delete(server.clients, key)
		}
	}
}

func (server *UDPServer) snapshotClients() []UDPClient {
	server.mu.Lock()
	defer server.mu.Unlock()

	clients := make([]UDPClient, 0, len(server.clients))
	for _, client := range server.clients {
		clients = append(clients, *client)
	}

	return clients
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
	buffer := make([]byte, 65535)

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
