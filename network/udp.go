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
	nextSequence      uint64
	mu                sync.Mutex
}

// UDPClient tracks a client address and authentication state.
type UDPClient struct {
	addr          *net.UDPAddr
	connectionID  string
	id            string
	authenticated bool
	wireProtocol  bool
	wireVersion   uint8
	lastSeq       uint64
	receivedSeqs  map[uint64]struct{}
	peerAck       uint64
	peerAckBits   uint64
	ack           uint64
	ackBits       uint64
	lastHeardAt   time.Time
}

type udpIncomingPacket struct {
	sequence uint64
	ack      uint64
	ackBits  uint64
	wire     bool
	version  uint8
	messages []udpIncomingMessage
}

type udpIncomingMessage struct {
	messageType WireMessageType
	request     RequestMessage
	payload     any
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

	incoming, err := server.decodeUDPIncomingPacket(message)
	if err != nil {
		server.recordRequestError()
		return err
	}

	client, accepted := server.acceptClientRequest(addr, incoming.sequence, incoming.ack, incoming.ackBits)
	if !accepted {
		return nil
	}
	if incoming.wire {
		server.markWireClient(client, incoming.version)
	}

	response := ResponseWriterFunc(func(responseMessage any) error {
		if conn == nil {
			return ErrResponsesUnsupported
		}

		sequence := server.nextOutgoingSequence()
		ack, ackBits := server.clientAckState(client)
		payload, err := server.encodeUDPResponse(responseMessage, sequence, ack, ackBits, incoming.wire)
		if err != nil {
			return err
		}

		_, err = conn.WriteToUDP(payload, addr)
		return err
	})

	var firstErr error
	for _, incomingMessage := range incoming.messages {
		if _, ok := incomingMessage.payload.(UnknownWireMessage); ok {
			continue
		}
		if incoming.wire {
			handled, err := server.runtime.WireMessages().Dispatch(WireMessageContext{
				Transport:  "udp",
				ClientID:   client.id,
				Sequence:   incoming.sequence,
				Ack:        incoming.ack,
				AckBits:    incoming.ackBits,
				ReceivedAt: time.Now(),
				MessageID:  incomingMessage.messageType,
				Message:    incomingMessage.payload,
				Runtime:    server.runtime,
				Response:   response,
			})
			if err != nil && firstErr == nil {
				firstErr = err
			}
			if handled {
				continue
			}
		}
		if emptyRequestMessage(incomingMessage.request) {
			continue
		}

		if err := server.handleDecodedRequest(client, incomingMessage.request, incomingMessage.payload, response); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func emptyRequestMessage(request RequestMessage) bool {
	return request.Type == "" &&
		request.Sequence == 0 &&
		request.ObjectType == "" &&
		request.ObjectID == "" &&
		request.Action == "" &&
		len(request.Data) == 0
}

// handleDecodedRequest routes a protocol-decoded request through auth or gameplay.
func (server *UDPServer) handleDecodedRequest(client *UDPClient, request RequestMessage, payload any, response ResponseWriter) error {
	if isAuthenticationRequest(request) {
		return server.handleAuthenticationAttempt(client, request, payload, response)
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
		Payload:    payload,
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
func (server *UDPServer) handleAuthenticationAttempt(client *UDPClient, request RequestMessage, payload any, response ResponseWriter) error {
	server.recordAuthAttempt()

	authenticatedID, err := server.runtime.HandleAuthenticationAttempt(AuthenticationContext{
		Transport:    "udp",
		ConnectionID: client.connectionID,
		ClientID:     client.id,
		ReceivedAt:   time.Now(),
		Request:      request,
		Payload:      payload,
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

func (server *UDPServer) decodeUDPIncomingPacket(payload []byte) (udpIncomingPacket, error) {
	if isWirePacket(payload) {
		return server.decodeUDPWirePacket(payload)
	}

	var request RequestMessage
	if err := json.Unmarshal(payload, &request); err != nil {
		return udpIncomingPacket{}, err
	}
	return udpIncomingPacket{
		sequence: request.Sequence,
		messages: []udpIncomingMessage{{
			request: request,
		}},
	}, nil
}

func (server *UDPServer) decodeUDPWirePacket(payload []byte) (udpIncomingPacket, error) {
	packet, err := DecodePacket(payload)
	if err != nil {
		return udpIncomingPacket{}, err
	}

	incoming := udpIncomingPacket{
		sequence: packet.Sequence,
		ack:      packet.Ack,
		ackBits:  packet.AckBits,
		wire:     true,
		version:  packet.Version,
		messages: make([]udpIncomingMessage, 0, len(packet.Messages)),
	}
	for _, message := range packet.Messages {
		decoded, err := server.runtime.WireMessages().DecodeMessage(message)
		if err != nil {
			return udpIncomingPacket{}, err
		}

		incomingMessage := udpIncomingMessage{
			messageType: message.Type,
			payload:     decoded,
		}
		switch value := decoded.(type) {
		case ClientHello:
			incomingMessage.request = RequestMessage{
				Type:     "auth",
				Sequence: packet.Sequence,
			}
		case ObjectRequest:
			incomingMessage.request = RequestMessage{
				Sequence:   packet.Sequence,
				ObjectType: value.ObjectType,
				ObjectID:   value.ObjectID,
				Action:     value.Action,
				Data:       json.RawMessage(value.Data),
			}
			incomingMessage.payload = nil
		case PlayerInput:
			incomingMessage.request = RequestMessage{
				Sequence:   packet.Sequence,
				ObjectType: "player",
				ObjectID:   value.ObjectID,
				Action:     "input",
			}
		}

		incoming.messages = append(incoming.messages, incomingMessage)
	}

	return incoming, nil
}

func isWirePacket(payload []byte) bool {
	return len(payload) >= 2 && payload[0] == byte(WireProtocolMagic>>8) && payload[1] == byte(WireProtocolMagic&0xff)
}

func (server *UDPServer) encodeUDPResponse(message any, sequence, ack, ackBits uint64, wire bool) ([]byte, error) {
	if !wire {
		return json.Marshal(message)
	}

	wireMessage, err := server.encodeUDPWireResponseMessage(message)
	if err != nil {
		return nil, err
	}
	return EncodePacketWithAcks(sequence, ack, ackBits, []WireMessage{wireMessage})
}

func (server *UDPServer) encodeUDPWireResponseMessage(message any) (WireMessage, error) {
	switch value := message.(type) {
	case AuthenticationResponse:
		return server.runtime.WireMessages().EncodeMessage(Welcome{
			ClientID:        value.ClientID,
			AuthenticatedID: value.AuthenticatedID,
		})
	case ResponseMessage:
		if value.OK {
			return server.runtime.WireMessages().EncodeMessage(ErrorMessage{Code: 0, Message: "ok"})
		}
		return server.runtime.WireMessages().EncodeMessage(ErrorMessage{Code: 1, Message: value.Error})
	default:
		return server.runtime.WireMessages().EncodeMessage(message)
	}
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
func (server *UDPServer) acceptClientRequest(addr *net.UDPAddr, sequence, ack, ackBits uint64) (*UDPClient, bool) {
	server.mu.Lock()
	defer server.mu.Unlock()

	client := server.getOrCreateClientLocked(addr)
	client.lastHeardAt = time.Now()
	client.peerAck = ack
	client.peerAckBits = ackBits

	if sequence == 0 {
		server.requestsAccepted++
		return client, true
	}
	if _, ok := client.receivedSeqs[sequence]; ok {
		server.requestsDropped++
		return client, false
	}
	if client.ack > 0 && sequence+64 <= client.ack {
		server.requestsDropped++
		return client, false
	}

	client.receivedSeqs[sequence] = struct{}{}
	if sequence > client.ack {
		client.ack = sequence
		client.lastSeq = sequence
	}
	client.ackBits = clientAckBitsLocked(client)
	pruneClientReceivedSeqsLocked(client)
	server.requestsAccepted++
	return client, true
}

func (server *UDPServer) nextOutgoingSequence() uint64 {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.nextSequence++
	return server.nextSequence
}

func (server *UDPServer) clientAckState(client *UDPClient) (uint64, uint64) {
	server.mu.Lock()
	defer server.mu.Unlock()

	return client.ack, client.ackBits
}

func clientAckBitsLocked(client *UDPClient) uint64 {
	if client.ack == 0 {
		return 0
	}

	var bits uint64
	for offset := uint64(0); offset < 64; offset++ {
		sequence := client.ack - offset - 1
		if _, ok := client.receivedSeqs[sequence]; ok {
			bits |= 1 << offset
		}
		if sequence == 0 {
			break
		}
	}
	return bits
}

func pruneClientReceivedSeqsLocked(client *UDPClient) {
	if client.ack <= 64 {
		return
	}
	minimum := client.ack - 64
	for sequence := range client.receivedSeqs {
		if sequence < minimum {
			delete(client.receivedSeqs, sequence)
		}
	}
}

func (server *UDPServer) markWireClient(client *UDPClient, version uint8) {
	server.mu.Lock()
	defer server.mu.Unlock()

	client.wireProtocol = true
	client.wireVersion = version
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
		receivedSeqs: map[uint64]struct{}{},
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
		sequence := server.nextOutgoingSequence()
		payload, err := server.encodeUDPSnapshot(client, sequence, tick, objects)
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

func (server *UDPServer) encodeUDPSnapshot(client UDPClient, sequence, tick uint64, objects SnapshotData) ([]byte, error) {
	if !client.wireProtocol {
		return json.Marshal(SnapshotMessage{
			Type:         "snapshot",
			ClientID:     client.id,
			Tick:         tick,
			LastSequence: client.lastSeq,
			Objects:      objects,
		})
	}

	message, err := server.runtime.WireMessages().EncodeMessage(WorldSnapshot{
		ClientID:     client.id,
		Tick:         tick,
		LastSequence: client.lastSeq,
		Objects:      objects,
	})
	if err != nil {
		return nil, err
	}
	return EncodePacketWithAcks(sequence, client.ack, client.ackBits, []WireMessage{message})
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
