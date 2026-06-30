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

// UDPServer serves the runtime protocol over UDP.
type UDPServer struct {
	address           string
	clients           map[string]*UDPClient
	runtime           *Runtime
	startedAt         time.Time
	datagramsReceived uint64
	requestsAccepted  uint64
	requestsDropped   uint64
	requestErrors     uint64
	authAttempts      uint64
	authSuccesses     uint64
	authFailures      uint64
	snapshotsSent     uint64
	snapshotErrors    uint64
	clientsCreated    uint64
	clientsRemoved    uint64
	mu                sync.Mutex
}

// UDPClient tracks a client address and authentication state.
type UDPClient struct {
	addr          *net.UDPAddr
	connectionID  string
	id            string
	authenticated bool
	lastSeq       uint64
	lastHeardAt   time.Time
}

// NewUDPServer creates a UDP transport for runtime.
func NewUDPServer(runtime *Runtime) *UDPServer {
	return &UDPServer{
		address:   runtime.Config().UDPAddress,
		clients:   map[string]*UDPClient{},
		runtime:   runtime,
		startedAt: time.Now(),
	}
}

// ListenAndServe binds the UDP socket and handles requests until the socket fails.
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

// handleMessage decodes a datagram and routes it through authentication or request handling.
func (server *UDPServer) handleMessage(conn *net.UDPConn, addr *net.UDPAddr, message []byte) error {
	server.recordDatagram()

	var request RequestMessage
	if err := json.Unmarshal(message, &request); err != nil {
		server.recordRequestError()
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
		server.recordRequestError()
		_ = response.Respond(ResponseMessage{
			Type:     "error",
			Sequence: request.Sequence,
			OK:       false,
			Error:    err.Error(),
		})
	}

	return err
}

// handleAuthenticationAttempt authenticates a client and sends the transport response.
func (server *UDPServer) handleAuthenticationAttempt(client *UDPClient, request RequestMessage, response ResponseWriter) error {
	server.recordAuthAttempt()

	authenticatedID, err := server.runtime.HandleAuthenticationAttempt(AuthenticationContext{
		Transport:    "udp",
		ConnectionID: client.connectionID,
		ClientID:     client.id,
		ReceivedAt:   time.Now(),
		Request:      request,
	})
	if err != nil {
		server.recordAuthFailure()
		_ = response.Respond(ResponseMessage{
			Type:     "error",
			Sequence: request.Sequence,
			OK:       false,
			Error:    err.Error(),
		})
		return err
	}

	if err := server.authenticateClient(client, authenticatedID); err != nil {
		server.recordAuthFailure()
		_ = response.Respond(ResponseMessage{
			Type:     "error",
			Sequence: request.Sequence,
			OK:       false,
			Error:    err.Error(),
		})
		return err
	}
	server.recordAuthSuccess()

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

// authenticateClient assigns an authenticated id if no other client owns it.
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

// acceptClientRequest updates client state and rejects stale sequenced requests.
func (server *UDPServer) acceptClientRequest(addr *net.UDPAddr, sequence uint64) (*UDPClient, bool) {
	server.mu.Lock()
	defer server.mu.Unlock()

	client := server.getOrCreateClientLocked(addr)
	client.lastHeardAt = time.Now()

	if sequence == 0 {
		server.requestsAccepted++
		return client, true
	}
	if sequence <= client.lastSeq {
		server.requestsDropped++
		return client, false
	}

	client.lastSeq = sequence
	server.requestsAccepted++
	return client, true
}

// getOrCreateClientLocked returns the client for addr while server.mu is held.
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
	server.clientsCreated++

	return client
}

// runTickLoop advances the runtime and broadcasts snapshots on each tick.
func (server *UDPServer) runTickLoop(conn *net.UDPConn) {
	ticker := time.NewTicker(server.runtime.Config().TickInterval())
	defer ticker.Stop()

	for range ticker.C {
		if err := server.advanceAndMaybeBroadcast(conn); err != nil {
			log.Println("udp snapshot error:", err)
		}
	}
}

// advanceAndMaybeBroadcast advances the runtime and sends snapshots on snapshot ticks.
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
			server.recordSnapshotError()
			log.Println("udp broadcast error:", err)
			continue
		}
		server.recordSnapshotSent()
	}

	return nil
}

type udpSnapshot struct {
	addr    *net.UDPAddr
	payload []byte
}

// snapshots builds one serialized snapshot per eligible client.
func (server *UDPServer) snapshots() ([]udpSnapshot, error) {
	tick := server.runtime.Tick()
	clients := server.snapshotClients()

	snapshots := make([]udpSnapshot, 0, len(clients))
	for _, client := range clients {
		objects := server.runtime.SnapshotForClient(client.id)
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

// removeStaleClients drops clients that have exceeded the configured timeout.
func (server *UDPServer) removeStaleClients(now time.Time) {
	server.mu.Lock()
	defer server.mu.Unlock()

	timeout := server.runtime.Config().ClientTimeout
	for key, client := range server.clients {
		if now.Sub(client.lastHeardAt) > timeout {
			delete(server.clients, key)
			server.clientsRemoved++
		}
	}
}

// snapshotClients returns clients eligible to receive snapshots.
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

// isAuthenticationRequest reports whether request should be routed to authentication.
func isAuthenticationRequest(request RequestMessage) bool {
	messageType := strings.ToLower(strings.TrimSpace(request.Type))
	return messageType == "auth" || messageType == "authentication"
}

// recordDatagram increments the received datagram counter.
func (server *UDPServer) recordDatagram() {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.datagramsReceived++
}

// recordRequestError increments the request error counter.
func (server *UDPServer) recordRequestError() {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.requestErrors++
}

// recordAuthAttempt increments the authentication attempt counter.
func (server *UDPServer) recordAuthAttempt() {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.authAttempts++
}

// recordAuthSuccess increments the authentication success counter.
func (server *UDPServer) recordAuthSuccess() {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.authSuccesses++
}

// recordAuthFailure increments the authentication failure counter.
func (server *UDPServer) recordAuthFailure() {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.authFailures++
}

// recordSnapshotSent increments the sent snapshot counter.
func (server *UDPServer) recordSnapshotSent() {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.snapshotsSent++
}

// recordSnapshotError increments the snapshot error counter.
func (server *UDPServer) recordSnapshotError() {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.snapshotErrors++
}
