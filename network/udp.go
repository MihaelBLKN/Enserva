package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

type UDPServer struct {
	address string
	clients map[string]*UDPClient
	runtime *Runtime
	mu      sync.Mutex
}

type UDPClient struct {
	addr          *net.UDPAddr
	connectionID  string
	id            string
	authenticated bool
	lastSeq       uint64
	lastHeardAt   time.Time
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

		if err := server.handleMessage(conn, clientAddr, buffer[:bytesRead]); err != nil {
			log.Println("udp request error:", err)
		}
	}
}

func (server *UDPServer) handleMessage(conn *net.UDPConn, addr *net.UDPAddr, message []byte) error {
	var request RequestMessage
	if err := json.Unmarshal(message, &request); err != nil {
		return err
	}

	client, accepted := server.acceptClientRequest(addr, request.Sequence)
	if !accepted {
		return nil
	}

	response := ResponseWriterFunc(func(message any) error {
		if conn == nil {
			return ErrResponsesUnsupported
		}

		payload, err := json.Marshal(message)
		if err != nil {
			return err
		}

		_, err = conn.WriteToUDP(payload, addr)
		return err
	})

	if isAuthenticationRequest(request) {
		return server.handleAuthenticationAttempt(client, request, response)
	}

	if server.runtime.AuthenticationRequired() && !client.authenticated {
		err := ErrAuthenticationRequired
		_ = response.Respond(ResponseMessage{
			Type:     "error",
			Sequence: request.Sequence,
			OK:       false,
			Error:    err.Error(),
		})
		return err
	}

	err := server.runtime.HandleRequest(RequestContext{
		Transport:  "udp",
		ClientID:   client.id,
		ReceivedAt: time.Now(),
		Request:    request,
		Response:   response,
	})
	if err != nil {
		_ = response.Respond(ResponseMessage{
			Type:     "error",
			Sequence: request.Sequence,
			OK:       false,
			Error:    err.Error(),
		})
	}

	return err
}

func (server *UDPServer) handleAuthenticationAttempt(client *UDPClient, request RequestMessage, response ResponseWriter) error {
	authenticatedID, err := server.runtime.HandleAuthenticationAttempt(AuthenticationContext{
		Transport:    "udp",
		ConnectionID: client.connectionID,
		ClientID:     client.id,
		ReceivedAt:   time.Now(),
		Request:      request,
	})
	if err != nil {
		_ = response.Respond(ResponseMessage{
			Type:     "error",
			Sequence: request.Sequence,
			OK:       false,
			Error:    err.Error(),
		})
		return err
	}

	if err := server.authenticateClient(client, authenticatedID); err != nil {
		_ = response.Respond(ResponseMessage{
			Type:     "error",
			Sequence: request.Sequence,
			OK:       false,
			Error:    err.Error(),
		})
		return err
	}

	err = response.Respond(AuthenticationResponse{
		Type:            "auth",
		Sequence:        request.Sequence,
		OK:              true,
		ClientID:        client.id,
		AuthenticatedID: authenticatedID,
	})
	if errors.Is(err, ErrResponsesUnsupported) {
		return nil
	}

	return err
}

func (server *UDPServer) authenticateClient(client *UDPClient, authenticatedID string) error {
	authenticatedID = strings.TrimSpace(authenticatedID)
	if authenticatedID == "" {
		return ErrMissingAuthenticationID
	}

	server.mu.Lock()
	defer server.mu.Unlock()

	for _, existing := range server.clients {
		if existing != client && existing.authenticated && existing.id == authenticatedID {
			return fmt.Errorf("%w: %s", ErrAuthenticatedClientIDInUse, authenticatedID)
		}
	}

	client.id = authenticatedID
	client.authenticated = true
	return nil
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
		addr:         addr,
		connectionID: "udp-" + key,
		id:           "udp-" + key,
		lastHeardAt:  time.Now(),
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

	authenticationRequired := server.runtime.AuthenticationRequired()
	clients := make([]UDPClient, 0, len(server.clients))
	for _, client := range server.clients {
		if authenticationRequired && !client.authenticated {
			continue
		}

		clients = append(clients, *client)
	}

	return clients
}

func isAuthenticationRequest(request RequestMessage) bool {
	messageType := strings.ToLower(strings.TrimSpace(request.Type))
	return messageType == "auth" || messageType == "authentication"
}
