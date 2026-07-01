package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// UDPServer serves the runtime protocol over UDP.
type UDPServer struct {
	address                       string
	clients                       map[string]*UDPClient
	runtime                       *Runtime
	startedAt                     time.Time
	datagramsReceived             uint64
	requestsAccepted              uint64
	requestsDropped               uint64
	requestErrors                 uint64
	authAttempts                  uint64
	authSuccesses                 uint64
	authFailures                  uint64
	snapshotsSent                 uint64
	fullSnapshotsSent             uint64
	deltaSnapshotsSent            uint64
	snapshotErrors                uint64
	oversizedOutbound             uint64
	reliableQueued                uint64
	reliableRetransmits           uint64
	reliableDrops                 uint64
	reliableAckRemovals           uint64
	budgetDrops                   uint64
	budgetDeferrals               uint64
	outboundBytesSent             uint64
	snapshotEncodeCount           uint64
	lastSnapshotEncodeDurationNs  int64
	maxSnapshotEncodeDurationNs   int64
	totalSnapshotEncodeDurationNs int64
	clientsCreated                uint64
	clientsRemoved                uint64
	nextSequence                  uint64
	snapshotBaselines             map[string]udpSnapshotBaseline
	mu                            sync.Mutex
}

// UDPClient tracks a client address and authentication state.
type UDPClient struct {
	addr                      *net.UDPAddr
	connectionID              string
	id                        string
	authenticated             bool
	wireProtocol              bool
	wireNegotiated            bool
	wireVersion               uint8
	wireCapabilities          WireCapabilities
	maxPacketSize             int
	lastSeq                   uint64
	receivedSeqs              map[uint64]struct{}
	peerAck                   uint64
	peerAckBits               uint64
	ack                       uint64
	ackBits                   uint64
	lastHeardAt               time.Time
	nextReliableID            uint64
	reliableOut               map[uint64]*udpReliableOutbound
	reliableUnorderedReceived map[uint64]struct{}
	reliableOrderedReceived   map[uint64]struct{}
	reliableOrderedNext       uint64
	reliableOrderedBuffer     map[uint64]udpIncomingMessage
	budgetAvailable           float64
	budgetLastRefill          time.Time
	budgetDrops               uint64
	budgetDeferrals           uint64
	bytesSent                 uint64
}

type udpSnapshotBaseline struct {
	tick               uint64
	snapshot           SnapshotData
	snapshotsSinceFull uint64
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
	delivery    DeliveryClass
	deliveryID  uint64
}

type udpReliableOutbound struct {
	message       WireMessage
	attempts      int
	lastSentAt    time.Time
	sentSequences map[uint64]struct{}
}

type udpReliableTransmit struct {
	client  *UDPClient
	addr    *net.UDPAddr
	payload []byte
}

// NewUDPServer creates a UDP transport for runtime.
func NewUDPServer(runtime *Runtime) *UDPServer {
	return &UDPServer{
		address:           runtime.Config().UDPAddress,
		clients:           map[string]*UDPClient{},
		runtime:           runtime,
		startedAt:         time.Now(),
		snapshotBaselines: map[string]udpSnapshotBaseline{},
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
		server.markWireClient(client)
	}

	response := ResponseWriterFunc(func(responseMessage any) error {
		if conn == nil {
			return ErrResponsesUnsupported
		}
		return server.sendUDPResponse(conn, addr, client, responseMessage, incoming.wire)
	})

	var firstErr error
	for _, incomingMessage := range server.acceptReliableIncomingMessages(client, incoming.messages) {
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
		handledInput, err := server.bufferDecodedInput(client, incoming.sequence, incomingMessage.payload)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if handledInput {
			continue
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
	} else if isSceneSwitchAction(request.Action) {
		server.resetSnapshotBaseline(client)
	}

	return err
}

// handleAuthenticationAttempt authenticates a client and sends the transport response.
func (server *UDPServer) handleAuthenticationAttempt(client *UDPClient, request RequestMessage, payload any, response ResponseWriter) error {
	server.recordAuthAttempt()
	if hello, ok := payload.(ClientHello); ok {
		welcome, err := NegotiateClientHello(server.runtime.Config(), hello)
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
		if hello.MaxPacketSize > 0 {
			server.applyClientPacketSize(client, hello.MaxPacketSize)
		}
		server.applyNegotiatedWireState(client, welcome)
	}

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
			delivery:    message.Delivery,
			deliveryID:  message.DeliveryID,
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
		case GenericClientInput:
			incomingMessage.request = RequestMessage{}
		}

		incoming.messages = append(incoming.messages, incomingMessage)
	}

	return incoming, nil
}

func (server *UDPServer) bufferDecodedInput(client *UDPClient, packetSequence uint64, payload any) (bool, error) {
	switch input := payload.(type) {
	case PlayerInput:
		sequence := input.Sequence
		if sequence == 0 {
			sequence = packetSequence
		}
		err := server.runtime.BufferClientInput(ClientInput{
			ClientID:   client.id,
			Sequence:   sequence,
			Tick:       input.Tick,
			ObjectType: "player",
			ObjectID:   input.ObjectID,
			TargetID:   input.ObjectID,
			Payload:    input,
			ReceivedAt: time.Now(),
		})
		return input.Tick != 0, err
	case GenericClientInput:
		sequence := input.Sequence
		if sequence == 0 {
			sequence = packetSequence
		}
		err := server.runtime.BufferClientInput(ClientInput{
			ClientID:   client.id,
			Sequence:   sequence,
			Tick:       input.Tick,
			ObjectType: input.ObjectType,
			ObjectID:   input.ObjectID,
			TargetID:   input.TargetID,
			Payload:    append([]byte(nil), input.Payload...),
			ReceivedAt: time.Now(),
		})
		return true, err
	default:
		return false, nil
	}
}

func isWirePacket(payload []byte) bool {
	return len(payload) >= 2 && payload[0] == byte(WireProtocolMagic>>8) && payload[1] == byte(WireProtocolMagic&0xff)
}

func (server *UDPServer) sendUDPResponse(conn *net.UDPConn, addr *net.UDPAddr, client *UDPClient, message any, wire bool) error {
	priority := server.outboundMessagePriority(message)
	if !wire {
		sequence := server.nextOutgoingSequence()
		payload, err := server.encodeUDPResponse(message, sequence, 0, 0, false)
		if err != nil {
			return err
		}
		if err := server.validateOutboundPacket("response", client, addr, payload); err != nil {
			return err
		}
		if !server.reserveOutboundBudget(client, len(payload), priority, false, time.Now()) {
			return nil
		}
		_, err = conn.WriteToUDP(payload, addr)
		if err == nil {
			server.recordOutboundBytes(client, len(payload))
		}
		return err
	}

	wireMessage, err := server.encodeUDPWireResponseMessage(client, message)
	if err != nil {
		return err
	}
	if err := server.prepareReliableMessage(client, &wireMessage); err != nil {
		return err
	}

	sequence := server.nextOutgoingSequence()
	ack, ackBits := server.clientAckState(client)
	payload, err := EncodePacketWithAcks(sequence, ack, ackBits, []WireMessage{wireMessage})
	if err != nil {
		return err
	}
	if err := server.validateOutboundPacket("response", client, addr, payload); err != nil {
		return err
	}
	if !server.reserveOutboundBudget(client, len(payload), priority, false, time.Now()) {
		return nil
	}
	if wireMessage.Delivery.reliable() {
		if err := server.queueReliableOutbound(client, wireMessage, sequence, time.Now()); err != nil {
			return err
		}
	}
	_, err = conn.WriteToUDP(payload, addr)
	if err == nil {
		server.recordOutboundBytes(client, len(payload))
	}
	return err
}

func (server *UDPServer) encodeUDPResponse(message any, sequence, ack, ackBits uint64, wire bool) ([]byte, error) {
	if priority, ok := message.(WirePriority); ok {
		return server.encodeUDPResponse(priority.Message, sequence, ack, ackBits, wire)
	}
	if !wire {
		return json.Marshal(message)
	}

	wireMessage, err := server.encodeUDPWireResponseMessage(nil, message)
	if err != nil {
		return nil, err
	}
	return EncodePacketWithAcks(sequence, ack, ackBits, []WireMessage{wireMessage})
}

func (server *UDPServer) encodeUDPWireResponseMessage(client *UDPClient, message any) (WireMessage, error) {
	if priority, ok := message.(WirePriority); ok {
		return server.encodeUDPWireResponseMessage(client, priority.Message)
	}
	if delivery, ok := message.(WireDelivery); ok {
		wireMessage, err := server.encodeUDPWireResponseMessage(client, delivery.Message)
		if err != nil {
			return WireMessage{}, err
		}
		wireMessage.Delivery = delivery.Class
		if client != nil && !server.clientSupportsReliable(client, delivery.Class) {
			wireMessage.Delivery = DeliveryUnreliable
		}
		return wireMessage, nil
	}

	switch value := message.(type) {
	case AuthenticationResponse:
		if client != nil {
			defaultPacketSize := server.runtime.Config().MaxUDPPacketSize
			if client.wireNegotiated || client.maxPacketSize > 0 && client.maxPacketSize != defaultPacketSize {
				return server.runtime.WireMessages().EncodeMessage(Welcome{
					ClientID:        value.ClientID,
					AuthenticatedID: value.AuthenticatedID,
					ProtocolVersion: client.wireVersion,
					Capabilities:    client.wireCapabilities,
					MaxPacketSize:   uint32(server.negotiatedMaxPacketSize(client)),
				})
			}
		}
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

func (server *UDPServer) validateOutboundPacket(kind string, client *UDPClient, addr *net.UDPAddr, payload []byte) error {
	limit := server.runtime.Config().MaxUDPPacketSize
	if negotiated := server.negotiatedMaxPacketSize(client); negotiated > 0 && negotiated < limit {
		limit = negotiated
	}
	if len(payload) <= limit {
		return nil
	}

	server.recordOversizedOutbound()
	target := "<unknown>"
	if addr != nil {
		target = addr.String()
	}
	log.Printf("udp %s dropped oversized outbound packet to %s: %d bytes exceeds max %d", kind, target, len(payload), limit)
	return fmt.Errorf("%w: %d bytes exceeds max %d", ErrUDPPacketTooLarge, len(payload), limit)
}

func (server *UDPServer) outboundBudgetLimit(client UDPClient, now time.Time) (int, bool) {
	config := server.runtime.Config()
	if !config.EnableBandwidthBudget {
		return 0, false
	}

	server.mu.Lock()
	defer server.mu.Unlock()

	live := server.clientByAddressLocked(client.addr)
	if live == nil {
		return 0, true
	}
	server.refillClientBudgetLocked(live, now, config)
	if live.budgetAvailable <= 0 {
		return 0, true
	}
	return int(live.budgetAvailable), true
}

func (server *UDPServer) reserveOutboundBudget(client *UDPClient, bytes int, priority OutboundPriority, deferrable bool, now time.Time) bool {
	if bytes <= 0 {
		return true
	}
	config := server.runtime.Config()
	if !config.EnableBandwidthBudget {
		return true
	}

	server.mu.Lock()
	defer server.mu.Unlock()

	live := server.clientByAddressLocked(clientAddr(client))
	if live == nil {
		return false
	}
	server.refillClientBudgetLocked(live, now, config)
	if live.budgetAvailable >= float64(bytes) {
		live.budgetAvailable -= float64(bytes)
		return true
	}

	if deferrable {
		server.budgetDeferrals++
		live.budgetDeferrals++
	} else {
		server.budgetDrops++
		live.budgetDrops++
	}
	_ = priority
	return false
}

func (server *UDPServer) refillClientBudgetLocked(client *UDPClient, now time.Time, config Config) {
	if client == nil || !config.EnableBandwidthBudget {
		return
	}
	capacity := float64(config.ClientBytesPerSecond)
	if capacity <= 0 {
		client.budgetAvailable = 0
		client.budgetLastRefill = now
		return
	}
	if client.budgetLastRefill.IsZero() {
		client.budgetAvailable = capacity
		client.budgetLastRefill = now
		return
	}
	if now.Before(client.budgetLastRefill) {
		client.budgetLastRefill = now
		return
	}
	elapsed := now.Sub(client.budgetLastRefill).Seconds()
	client.budgetAvailable += elapsed * capacity
	if client.budgetAvailable > capacity {
		client.budgetAvailable = capacity
	}
	client.budgetLastRefill = now
}

func (server *UDPServer) clientByAddressLocked(addr *net.UDPAddr) *UDPClient {
	if addr == nil {
		return nil
	}
	return server.clients[addr.String()]
}

func clientAddr(client *UDPClient) *net.UDPAddr {
	if client == nil {
		return nil
	}
	return client.addr
}

func (server *UDPServer) recordOutboundBytes(client *UDPClient, bytes int) {
	if client == nil || bytes <= 0 {
		return
	}

	server.mu.Lock()
	defer server.mu.Unlock()

	live := server.clientByAddressLocked(client.addr)
	if live != nil {
		live.bytesSent += uint64(bytes)
	}
	server.outboundBytesSent += uint64(bytes)
}

func (server *UDPServer) recordBudgetDeferral(client *UDPClient) {
	server.mu.Lock()
	defer server.mu.Unlock()

	if live := server.clientByAddressLocked(clientAddr(client)); live != nil {
		live.budgetDeferrals++
	}
	server.budgetDeferrals++
}

func (server *UDPServer) recordSnapshotDeferrals(client *UDPClient, count uint64) {
	if count == 0 {
		return
	}
	server.mu.Lock()
	defer server.mu.Unlock()

	if live := server.clientByAddressLocked(clientAddr(client)); live != nil {
		live.budgetDeferrals += count
	}
	server.budgetDeferrals += count
}

func (server *UDPServer) outboundMessagePriority(message any) OutboundPriority {
	switch value := message.(type) {
	case WirePriority:
		return value.Priority
	case WireDelivery:
		return server.outboundMessagePriority(value.Message)
	case AuthenticationResponse, Welcome, ErrorMessage, DisconnectMessage:
		return OutboundPriorityEssential
	case Pong:
		return OutboundPriorityHigh
	case ResponseMessage:
		if !value.OK {
			return OutboundPriorityEssential
		}
		return OutboundPriorityHigh
	default:
		return OutboundPriorityNormal
	}
}

func (server *UDPServer) applyNegotiatedWireState(client *UDPClient, welcome Welcome) {
	if client == nil {
		return
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	client.wireNegotiated = welcome.ProtocolVersion != 0 || welcome.Capabilities != 0

	if welcome.ProtocolVersion != 0 {
		client.wireVersion = welcome.ProtocolVersion
	}
	if client.wireNegotiated {
		client.wireCapabilities = welcome.Capabilities
	}
	if welcome.MaxPacketSize > 0 {
		client.maxPacketSize = int(welcome.MaxPacketSize)
	}
}

func (server *UDPServer) applyClientPacketSize(client *UDPClient, packetSize uint32) {
	if client == nil || packetSize == 0 {
		return
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	limit := server.runtime.Config().MaxUDPPacketSize
	if packetSize < uint32(limit) {
		client.maxPacketSize = int(packetSize)
		return
	}
	client.maxPacketSize = limit
}

func (server *UDPServer) negotiatedMaxPacketSize(client *UDPClient) int {
	if client == nil || client.maxPacketSize <= 0 {
		return server.runtime.Config().MaxUDPPacketSize
	}
	serverLimit := server.runtime.Config().MaxUDPPacketSize
	if client.maxPacketSize < serverLimit {
		return client.maxPacketSize
	}
	return serverLimit
}

func (server *UDPServer) clientSupportsReliable(client *UDPClient, class DeliveryClass) bool {
	if client == nil || !class.reliable() {
		return true
	}
	if !client.wireNegotiated {
		return true
	}
	if client.wireCapabilities&WireCapabilityReliableOrdered != 0 && class == DeliveryReliableOrdered {
		return true
	}
	if client.wireCapabilities&WireCapabilityReliableUnordered != 0 && class == DeliveryReliableUnordered {
		return true
	}
	return false
}

func (server *UDPServer) clientSupportsDeltaSnapshots(client *UDPClient) bool {
	if client == nil || !client.wireNegotiated {
		return true
	}
	return client.wireCapabilities&WireCapabilityDeltaSnapshots != 0
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
	delete(server.snapshotBaselines, client.connectionID)
	return nil
}

// acceptClientRequest updates client state and rejects stale sequenced requests.
func (server *UDPServer) acceptClientRequest(addr *net.UDPAddr, sequence, ack, ackBits uint64) (*UDPClient, bool) {
	server.mu.Lock()
	defer server.mu.Unlock()

	client := server.clientByAddressLocked(addr)
	if client == nil && server.runtime.Config().MaxClients > 0 && len(server.clients) >= server.runtime.Config().MaxClients {
		server.requestsDropped++
		return nil, false
	}

	client = server.getOrCreateClientLocked(addr)
	client.lastHeardAt = time.Now()
	client.peerAck = ack
	client.peerAckBits = ackBits
	server.removeAckedReliableLocked(client, ack, ackBits)

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

	return server.nextOutgoingSequenceLocked()
}

func (server *UDPServer) nextOutgoingSequenceLocked() uint64 {
	server.nextSequence++
	return server.nextSequence
}

func (server *UDPServer) clientAckState(client *UDPClient) (uint64, uint64) {
	server.mu.Lock()
	defer server.mu.Unlock()

	return client.ack, client.ackBits
}

func (server *UDPServer) prepareReliableMessage(client *UDPClient, message *WireMessage) error {
	if client == nil || message == nil || !message.Delivery.reliable() {
		return nil
	}
	if !server.clientSupportsReliable(client, message.Delivery) {
		message.Delivery = DeliveryUnreliable
		message.DeliveryID = 0
		return nil
	}

	server.mu.Lock()
	defer server.mu.Unlock()

	if len(client.reliableOut) >= server.runtime.Config().ReliableQueueLimit {
		server.reliableDrops++
		return ErrReliableQueueFull
	}
	client.nextReliableID++
	message.DeliveryID = client.nextReliableID
	return nil
}

func (server *UDPServer) queueReliableOutbound(client *UDPClient, message WireMessage, sequence uint64, sentAt time.Time) error {
	if client == nil || !message.Delivery.reliable() {
		return nil
	}
	if message.DeliveryID == 0 {
		return ErrReliableMessageID
	}

	server.mu.Lock()
	defer server.mu.Unlock()

	if len(client.reliableOut) >= server.runtime.Config().ReliableQueueLimit {
		server.reliableDrops++
		return ErrReliableQueueFull
	}
	client.reliableOut[message.DeliveryID] = &udpReliableOutbound{
		message:    message,
		attempts:   1,
		lastSentAt: sentAt,
		sentSequences: map[uint64]struct{}{
			sequence: {},
		},
	}
	server.reliableQueued++
	return nil
}

func (server *UDPServer) removeAckedReliableLocked(client *UDPClient, ack, ackBits uint64) {
	if client == nil || len(client.reliableOut) == 0 {
		return
	}

	for deliveryID, queued := range client.reliableOut {
		for sequence := range queued.sentSequences {
			if packetSequenceAcked(ack, ackBits, sequence) {
				delete(client.reliableOut, deliveryID)
				server.reliableAckRemovals++
				break
			}
		}
	}
}

func (server *UDPServer) retransmitReliable(conn *net.UDPConn, now time.Time) {
	if conn == nil {
		return
	}

	for _, transmit := range server.collectReliableRetransmits(now) {
		if err := server.validateOutboundPacket("reliable retry", transmit.client, transmit.addr, transmit.payload); err != nil {
			server.recordReliableDrop()
			continue
		}
		if !server.reserveOutboundBudget(transmit.client, len(transmit.payload), OutboundPriorityHigh, true, now) {
			continue
		}
		if _, err := conn.WriteToUDP(transmit.payload, transmit.addr); err != nil {
			server.recordReliableDrop()
			log.Println("udp reliable retry error:", err)
		} else {
			server.recordOutboundBytes(transmit.client, len(transmit.payload))
		}
	}
}

func (server *UDPServer) collectReliableRetransmits(now time.Time) []udpReliableTransmit {
	server.mu.Lock()
	defer server.mu.Unlock()

	config := server.runtime.Config()
	transmits := make([]udpReliableTransmit, 0)
	for _, client := range server.clients {
		for deliveryID, queued := range client.reliableOut {
			if now.Sub(queued.lastSentAt) < config.ReliableRetryInterval {
				continue
			}
			if queued.attempts >= config.ReliableMaxAttempts {
				delete(client.reliableOut, deliveryID)
				server.reliableDrops++
				continue
			}

			sequence := server.nextOutgoingSequenceLocked()
			payload, err := EncodePacketWithAcks(sequence, client.ack, client.ackBits, []WireMessage{queued.message})
			if err != nil {
				delete(client.reliableOut, deliveryID)
				server.reliableDrops++
				continue
			}
			queued.attempts++
			queued.lastSentAt = now
			queued.sentSequences[sequence] = struct{}{}
			server.reliableRetransmits++
			transmits = append(transmits, udpReliableTransmit{
				client:  client,
				addr:    client.addr,
				payload: payload,
			})
		}
	}
	return transmits
}

func (server *UDPServer) acceptReliableIncomingMessages(client *UDPClient, messages []udpIncomingMessage) []udpIncomingMessage {
	if len(messages) == 0 {
		return nil
	}

	server.mu.Lock()
	defer server.mu.Unlock()

	accepted := make([]udpIncomingMessage, 0, len(messages))
	for _, message := range messages {
		switch message.delivery {
		case DeliveryUnreliable:
			accepted = append(accepted, message)
		case DeliveryReliableUnordered:
			if !server.clientSupportsReliable(client, message.delivery) {
				server.reliableDrops++
				continue
			}
			if message.deliveryID == 0 {
				server.reliableDrops++
				continue
			}
			if _, duplicate := client.reliableUnorderedReceived[message.deliveryID]; duplicate {
				server.requestsDropped++
				continue
			}
			client.reliableUnorderedReceived[message.deliveryID] = struct{}{}
			accepted = append(accepted, message)
		case DeliveryReliableOrdered:
			if !server.clientSupportsReliable(client, message.delivery) {
				server.reliableDrops++
				continue
			}
			if message.deliveryID == 0 {
				server.reliableDrops++
				continue
			}
			if _, duplicate := client.reliableOrderedReceived[message.deliveryID]; duplicate {
				server.requestsDropped++
				continue
			}
			if _, duplicate := client.reliableOrderedBuffer[message.deliveryID]; duplicate {
				server.requestsDropped++
				continue
			}
			if message.deliveryID > client.reliableOrderedNext {
				client.reliableOrderedBuffer[message.deliveryID] = message
				continue
			}
			if message.deliveryID < client.reliableOrderedNext {
				server.requestsDropped++
				continue
			}

			accepted = append(accepted, message)
			client.reliableOrderedReceived[message.deliveryID] = struct{}{}
			client.reliableOrderedNext++
			for {
				buffered, ok := client.reliableOrderedBuffer[client.reliableOrderedNext]
				if !ok {
					break
				}
				delete(client.reliableOrderedBuffer, client.reliableOrderedNext)
				accepted = append(accepted, buffered)
				client.reliableOrderedReceived[buffered.deliveryID] = struct{}{}
				client.reliableOrderedNext++
			}
		default:
			server.reliableDrops++
		}
	}

	return accepted
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

func (server *UDPServer) markWireClient(client *UDPClient) {
	server.mu.Lock()
	defer server.mu.Unlock()

	if !client.wireProtocol {
		delete(server.snapshotBaselines, client.connectionID)
	}
	client.wireProtocol = true
	if client.wireCapabilities == 0 {
		client.wireCapabilities = DefaultWireCapabilities()
	}
	if client.wireVersion == 0 {
		client.wireVersion = WireProtocolVersion
	}
	if client.maxPacketSize == 0 {
		client.maxPacketSize = server.runtime.Config().MaxUDPPacketSize
	}
}

// getOrCreateClientLocked returns the client for addr while server.mu is held.
func (server *UDPServer) getOrCreateClientLocked(addr *net.UDPAddr) *UDPClient {
	key := addr.String()
	client, ok := server.clients[key]
	if ok {
		return client
	}

	client = &UDPClient{
		addr:                      addr,
		connectionID:              "udp-" + key,
		id:                        "udp-" + key,
		receivedSeqs:              map[uint64]struct{}{},
		lastHeardAt:               time.Now(),
		reliableOut:               map[uint64]*udpReliableOutbound{},
		reliableUnorderedReceived: map[uint64]struct{}{},
		reliableOrderedReceived:   map[uint64]struct{}{},
		reliableOrderedNext:       1,
		reliableOrderedBuffer:     map[uint64]udpIncomingMessage{},
		maxPacketSize:             server.runtime.Config().MaxUDPPacketSize,
	}
	config := server.runtime.Config()
	if config.EnableBandwidthBudget {
		client.budgetAvailable = float64(config.ClientBytesPerSecond)
		client.budgetLastRefill = time.Now()
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
	now := time.Now()
	server.removeStaleClients(now)
	server.retransmitReliable(conn, now)

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
		server.recordOutboundBytes(&snapshot.client, len(snapshot.payload))
		server.recordSnapshotSent(snapshot.kind)
	}

	return nil
}

type udpSnapshot struct {
	addr    *net.UDPAddr
	payload []byte
	kind    string
	client  UDPClient
}

// snapshots builds one serialized snapshot per eligible client.
func (server *UDPServer) snapshots() ([]udpSnapshot, error) {
	tick := server.runtime.Tick()
	clients := server.snapshotClients()
	now := time.Now()

	snapshots := make([]udpSnapshot, 0, len(clients))
	for _, client := range clients {
		objects := server.runtime.SnapshotForClient(client.id)
		sequence := server.nextOutgoingSequence()
		payload, kind, filteredObjects, err := server.encodeBudgetedUDPSnapshot(client, sequence, tick, objects, now)
		if err != nil {
			return nil, err
		}
		if len(payload) == 0 {
			continue
		}
		if err := server.validateOutboundPacket("snapshot", &client, client.addr, payload); err != nil {
			if errors.Is(err, ErrUDPPacketTooLarge) {
				continue
			}
			return nil, err
		}

		server.commitSnapshotBaseline(client, tick, filteredObjects, kind)
		snapshots = append(snapshots, udpSnapshot{
			addr:    client.addr,
			payload: payload,
			kind:    kind,
			client:  client,
		})
	}

	return snapshots, nil
}

func (server *UDPServer) encodeBudgetedUDPSnapshot(client UDPClient, sequence, tick uint64, objects SnapshotData, now time.Time) ([]byte, string, SnapshotData, error) {
	payload, kind, err := server.encodeUDPSnapshot(client, sequence, tick, objects)
	if err != nil {
		return nil, "", nil, err
	}

	budgetLimit, budgeting := server.outboundBudgetLimit(client, now)
	if !budgeting {
		return payload, kind, objects, nil
	}

	limit := server.negotiatedMaxPacketSize(&client)
	if budgetLimit < limit {
		limit = budgetLimit
	}
	if len(payload) <= limit {
		if !server.reserveOutboundBudget(&client, len(payload), server.runtime.Config().DefaultSnapshotPriority, true, now) {
			return nil, "", nil, nil
		}
		return payload, kind, objects, nil
	}

	filtered := server.filterSnapshotForLimit(client, sequence, tick, objects, limit)
	if filtered.dropped == 0 {
		if len(payload) > server.negotiatedMaxPacketSize(&client) {
			server.recordOversizedOutbound()
		} else {
			server.recordBudgetDeferral(&client)
		}
		return nil, "", nil, nil
	}
	if filtered.payload == nil {
		return nil, "", nil, filtered.err
	}
	if !server.reserveOutboundBudget(&client, len(filtered.payload), server.runtime.Config().DefaultSnapshotPriority, true, now) {
		return nil, "", nil, nil
	}
	server.recordSnapshotDeferrals(&client, uint64(filtered.dropped))
	return filtered.payload, filtered.kind, filtered.objects, nil
}

type udpFilteredSnapshot struct {
	payload []byte
	kind    string
	objects SnapshotData
	dropped int
	err     error
}

func (server *UDPServer) filterSnapshotForLimit(client UDPClient, sequence, tick uint64, objects SnapshotData, limit int) udpFilteredSnapshot {
	if limit <= 0 {
		return udpFilteredSnapshot{}
	}

	filtered := CloneSnapshotData(objects)
	candidates := server.snapshotDropCandidates(objects)
	dropped := 0
	for _, candidate := range candidates {
		removeSnapshotObject(filtered, candidate.objectType, candidate.objectID)
		dropped++
		payload, kind, err := server.encodeUDPSnapshot(client, sequence, tick, filtered)
		if err != nil {
			return udpFilteredSnapshot{err: err}
		}
		if len(payload) <= limit {
			return udpFilteredSnapshot{
				payload: payload,
				kind:    kind,
				objects: filtered,
				dropped: dropped,
			}
		}
	}

	payload, kind, err := server.encodeUDPSnapshot(client, sequence, tick, filtered)
	if err != nil {
		return udpFilteredSnapshot{err: err}
	}
	if len(payload) <= limit {
		return udpFilteredSnapshot{
			payload: payload,
			kind:    kind,
			objects: filtered,
			dropped: dropped,
		}
	}
	return udpFilteredSnapshot{dropped: dropped}
}

type udpSnapshotDropCandidate struct {
	objectType string
	objectID   string
	priority   OutboundPriority
}

func (server *UDPServer) snapshotDropCandidates(objects SnapshotData) []udpSnapshotDropCandidate {
	priorities := server.runtime.SnapshotPriorities(objects)
	defaultPriority := server.runtime.Config().DefaultSnapshotPriority
	candidates := make([]udpSnapshotDropCandidate, 0)
	for objectType, objectsByID := range objects {
		for objectID := range objectsByID {
			priority, ok := priorities[snapshotPriorityKey(objectType, objectID)]
			if !ok {
				priority = defaultPriority
			}
			candidates = append(candidates, udpSnapshotDropCandidate{
				objectType: objectType,
				objectID:   objectID,
				priority:   priority,
			})
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].priority == candidates[right].priority {
			if candidates[left].objectType == candidates[right].objectType {
				return candidates[left].objectID > candidates[right].objectID
			}
			return candidates[left].objectType > candidates[right].objectType
		}
		return candidates[left].priority < candidates[right].priority
	})
	return candidates
}

func removeSnapshotObject(snapshot SnapshotData, objectType, objectID string) {
	objectsByID := snapshot[objectType]
	if objectsByID == nil {
		return
	}
	delete(objectsByID, objectID)
	if len(objectsByID) == 0 {
		delete(snapshot, objectType)
	}
}

func snapshotPriorityKey(objectType, objectID string) string {
	return objectType + "\x00" + objectID
}

func (server *UDPServer) encodeUDPSnapshot(client UDPClient, sequence, tick uint64, objects SnapshotData) ([]byte, string, error) {
	startedAt := time.Now()
	defer func() {
		server.recordSnapshotEncodeDuration(time.Since(startedAt))
	}()

	kind := "full"
	baseline, hasBaseline := server.snapshotBaseline(client.connectionID)
	if server.runtime.Config().EnableDeltaSnapshots && hasBaseline && !server.shouldSendFullSnapshot(baseline) && server.clientSupportsDeltaSnapshots(&client) {
		kind = "delta"
	}

	if kind == "delta" {
		delta := BuildSnapshotDelta(baseline.snapshot, objects)
		return server.encodeUDPDeltaSnapshot(client, sequence, tick, baseline.tick, delta)
	}

	if !client.wireProtocol {
		payload, err := json.Marshal(SnapshotMessage{
			Type:         "snapshot",
			ClientID:     client.id,
			Tick:         tick,
			LastSequence: client.lastSeq,
			Objects:      objects,
		})
		return payload, kind, err
	}

	message, err := server.runtime.WireMessages().EncodeMessage(WorldSnapshot{
		ClientID:     client.id,
		Tick:         tick,
		LastSequence: client.lastSeq,
		Objects:      objects,
	})
	if err != nil {
		return nil, "", err
	}
	payload, err := EncodePacketWithAcks(sequence, client.ack, client.ackBits, []WireMessage{message})
	return payload, kind, err
}

func (server *UDPServer) encodeUDPDeltaSnapshot(client UDPClient, sequence, tick, baselineTick uint64, delta SnapshotDelta) ([]byte, string, error) {
	kind := "delta"
	if !client.wireProtocol {
		payload, err := json.Marshal(DeltaSnapshotMessage{
			Type:         "snapshot.delta",
			ClientID:     client.id,
			Tick:         tick,
			LastSequence: client.lastSeq,
			BaselineTick: baselineTick,
			Spawned:      delta.Spawned,
			Changed:      delta.Changed,
			Despawned:    delta.Despawned,
		})
		return payload, kind, err
	}

	message, err := server.runtime.WireMessages().EncodeMessage(WorldDeltaSnapshot{
		ClientID:     client.id,
		Tick:         tick,
		LastSequence: client.lastSeq,
		BaselineTick: baselineTick,
		Spawned:      delta.Spawned,
		Changed:      delta.Changed,
		Despawned:    delta.Despawned,
	})
	if err != nil {
		return nil, "", err
	}
	payload, err := EncodePacketWithAcks(sequence, client.ack, client.ackBits, []WireMessage{message})
	return payload, kind, err
}

func (server *UDPServer) snapshotBaseline(connectionID string) (udpSnapshotBaseline, bool) {
	server.mu.Lock()
	defer server.mu.Unlock()

	baseline, ok := server.snapshotBaselines[connectionID]
	return baseline, ok
}

func (server *UDPServer) shouldSendFullSnapshot(baseline udpSnapshotBaseline) bool {
	interval := server.runtime.Config().FullSnapshotEvery()
	if interval <= 1 {
		return true
	}
	return baseline.snapshotsSinceFull+1 >= interval
}

func (server *UDPServer) commitSnapshotBaseline(client UDPClient, tick uint64, objects SnapshotData, kind string) {
	if !server.runtime.Config().EnableDeltaSnapshots {
		return
	}

	server.mu.Lock()
	defer server.mu.Unlock()

	snapshotsSinceFull := uint64(0)
	if kind == "delta" {
		if previous, ok := server.snapshotBaselines[client.connectionID]; ok {
			snapshotsSinceFull = previous.snapshotsSinceFull + 1
		}
	}
	server.snapshotBaselines[client.connectionID] = udpSnapshotBaseline{
		tick:               tick,
		snapshot:           CloneSnapshotData(objects),
		snapshotsSinceFull: snapshotsSinceFull,
	}
}

func (server *UDPServer) resetSnapshotBaseline(client *UDPClient) {
	if client == nil {
		return
	}

	server.mu.Lock()
	defer server.mu.Unlock()

	delete(server.snapshotBaselines, client.connectionID)
}

// removeStaleClients drops clients that have exceeded the configured timeout.
func (server *UDPServer) removeStaleClients(now time.Time) {
	server.mu.Lock()
	defer server.mu.Unlock()

	timeout := server.runtime.Config().ClientTimeout
	for key, client := range server.clients {
		if now.Sub(client.lastHeardAt) > timeout {
			delete(server.snapshotBaselines, client.connectionID)
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

// recordSnapshotSent increments the sent snapshot counters.
func (server *UDPServer) recordSnapshotSent(kind string) {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.snapshotsSent++
	if kind == "delta" {
		server.deltaSnapshotsSent++
		return
	}
	server.fullSnapshotsSent++
}

// recordSnapshotError increments the snapshot error counter.
func (server *UDPServer) recordSnapshotError() {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.snapshotErrors++
}

// recordOversizedOutbound increments the oversized outbound packet drop counter.
func (server *UDPServer) recordOversizedOutbound() {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.oversizedOutbound++
}

func (server *UDPServer) recordReliableDrop() {
	server.mu.Lock()
	defer server.mu.Unlock()

	server.reliableDrops++
}

func (server *UDPServer) recordSnapshotEncodeDuration(duration time.Duration) {
	if duration < 0 {
		return
	}

	nanoseconds := duration.Nanoseconds()
	if nanoseconds == 0 {
		nanoseconds = 1
	}
	server.mu.Lock()
	defer server.mu.Unlock()

	server.snapshotEncodeCount++
	server.lastSnapshotEncodeDurationNs = nanoseconds
	server.totalSnapshotEncodeDurationNs += nanoseconds
	if nanoseconds > server.maxSnapshotEncodeDurationNs {
		server.maxSnapshotEncodeDurationNs = nanoseconds
	}
}

func snapshotAverageDurationMillis(totalNanoseconds int64, count uint64) float64 {
	if count == 0 {
		return 0
	}
	return durationMillis(totalNanoseconds) / float64(count)
}
