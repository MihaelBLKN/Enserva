package tests

import (
	"Enserva/network"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"
)

const reliabilityMessageID = network.WireMessageGameMin + 30

type reliabilityObject struct {
	mu        sync.Mutex
	lastError error
}

func (object *reliabilityObject) ObjectType() string { return "reliability" }
func (object *reliabilityObject) ObjectID() string   { return "target" }
func (object *reliabilityObject) Snapshot() any      { return map[string]any{"ok": true} }

func (object *reliabilityObject) OnRequest(ctx network.RequestContext) error {
	switch ctx.Request.Action {
	case "reliable":
		object.recordError(ctx.Respond(network.DeliverReliableOrdered(network.Pong{Nonce: ctx.Request.Sequence})))
	case "queue-limit":
		object.recordError(ctx.Respond(network.DeliverReliableOrdered(network.Pong{Nonce: 1})))
		object.recordError(ctx.Respond(network.DeliverReliableOrdered(network.Pong{Nonce: 2})))
	default:
		object.recordError(ctx.Respond(network.Pong{Nonce: ctx.Request.Sequence}))
	}
	return nil
}

func (object *reliabilityObject) recordError(err error) {
	object.mu.Lock()
	defer object.mu.Unlock()
	if err != nil {
		object.lastError = err
	}
}

func (object *reliabilityObject) LastError() error {
	object.mu.Lock()
	defer object.mu.Unlock()
	return object.lastError
}

type reliabilityClientMessage struct {
	Number uint64
}

func TestReliableWireEnvelopeRoundTripExternal(t *testing.T) {
	message, err := network.EncodeClientMessage(network.Ping{Nonce: 42})
	if err != nil {
		t.Fatalf("encode ping: %v", err)
	}
	message.Delivery = network.DeliveryReliableOrdered
	message.DeliveryID = 7

	payload, err := network.EncodePacket(1, []network.WireMessage{message})
	if err != nil {
		t.Fatalf("encode packet: %v", err)
	}
	packet, err := network.DecodePacket(payload)
	if err != nil {
		t.Fatalf("decode packet: %v", err)
	}
	if len(packet.Messages) != 1 {
		t.Fatalf("expected one message, got %d", len(packet.Messages))
	}
	decoded := packet.Messages[0]
	if decoded.Type != network.WireMessagePing || decoded.Delivery != network.DeliveryReliableOrdered || decoded.DeliveryID != 7 {
		t.Fatalf("unexpected decoded reliable message: %#v", decoded)
	}
}

func TestAckedReliableMessagesAreRemovedFromRetryQueueExternal(t *testing.T) {
	server, addr, _ := startReliabilityServer(t, network.Config{
		TickRate:              200,
		SnapshotRate:          1,
		ReliableRetryInterval: time.Hour,
	})
	conn := dialUDPPacketSizeClient(t, addr)
	defer conn.Close()

	sendReliabilityObjectRequest(t, conn, 1, 0, 0, "reliable")
	response := readWireMessageType(t, conn, network.WireMessagePong)
	if response.message.Delivery != network.DeliveryReliableOrdered {
		t.Fatalf("expected reliable ordered response, got %#v", response.message)
	}

	sendReliabilityPing(t, conn, 2, response.packet.Sequence, 0)
	waitForReliableCounter(t, server, func(counters network.DebugUDPCounters) bool {
		return counters.ReliableAckRemovals >= 1
	})

	state := server.DebugState()
	if len(state.UDP.Clients) != 1 || state.UDP.Clients[0].ReliableQueued != 0 {
		t.Fatalf("expected acked reliable queue to be empty, got %#v", state.UDP.Clients)
	}
}

func TestUnackedReliableMessagesRetransmitAfterRetryIntervalExternal(t *testing.T) {
	server, addr, _ := startReliabilityServer(t, network.Config{
		TickRate:              500,
		SnapshotRate:          1,
		ReliableRetryInterval: time.Millisecond,
		ReliableMaxAttempts:   4,
	})
	conn := dialUDPPacketSizeClient(t, addr)
	defer conn.Close()

	sendReliabilityObjectRequest(t, conn, 1, 0, 0, "reliable")
	first := readWireMessageType(t, conn, network.WireMessagePong)
	second := readWireMessageType(t, conn, network.WireMessagePong)

	if first.message.DeliveryID == 0 || second.message.DeliveryID != first.message.DeliveryID {
		t.Fatalf("expected retry with same reliable id, first=%#v second=%#v", first.message, second.message)
	}
	if first.packet.Sequence == second.packet.Sequence {
		t.Fatalf("expected retry packet to use a new packet sequence")
	}
	waitForReliableCounter(t, server, func(counters network.DebugUDPCounters) bool {
		return counters.ReliableRetransmits >= 1
	})
}

func TestReliableMaxAttemptsAndQueueLimitAreEnforcedExternal(t *testing.T) {
	server, addr, object := startReliabilityServer(t, network.Config{
		TickRate:              500,
		SnapshotRate:          1,
		ReliableRetryInterval: time.Hour,
		ReliableMaxAttempts:   4,
		ReliableQueueLimit:    1,
	})
	conn := dialUDPPacketSizeClient(t, addr)
	defer conn.Close()

	sendReliabilityObjectRequest(t, conn, 1, 0, 0, "queue-limit")
	_ = readWireMessageType(t, conn, network.WireMessagePong)
	waitForReliableCounter(t, server, func(counters network.DebugUDPCounters) bool {
		return counters.ReliableDrops >= 1
	})
	if !errors.Is(object.LastError(), network.ErrReliableQueueFull) {
		t.Fatalf("expected queue-limit response error, got %v", object.LastError())
	}

	maxServer, maxAddr, _ := startReliabilityServer(t, network.Config{
		TickRate:              500,
		SnapshotRate:          1,
		ReliableRetryInterval: time.Millisecond,
		ReliableMaxAttempts:   1,
		ReliableQueueLimit:    4,
	})
	maxConn := dialUDPPacketSizeClient(t, maxAddr)
	defer maxConn.Close()

	sendReliabilityObjectRequest(t, maxConn, 1, 0, 0, "reliable")
	_ = readWireMessageType(t, maxConn, network.WireMessagePong)
	waitForReliableCounter(t, maxServer, func(counters network.DebugUDPCounters) bool {
		return counters.ReliableDrops >= 1
	})
	state := maxServer.DebugState()
	if len(state.UDP.Clients) != 1 || state.UDP.Clients[0].ReliableQueued != 0 {
		t.Fatalf("expected max-attempt drop to empty queue, got %#v", state.UDP.Clients)
	}
}

func TestDuplicateReliableInboundMessageIsSuppressedExternal(t *testing.T) {
	_, addr, recorder := startReliabilityHandlerServer(t)
	conn := dialUDPPacketSizeClient(t, addr)
	defer conn.Close()

	sendReliabilityClientMessage(t, conn, 1, 0, 0, 1, 11, network.DeliveryReliableUnordered)
	sendReliabilityClientMessage(t, conn, 2, 0, 0, 1, 11, network.DeliveryReliableUnordered)

	waitForRecordedMessages(t, recorder, []uint64{11})
	time.Sleep(20 * time.Millisecond)
	if got := recorder.Values(); len(got) != 1 || got[0] != 11 {
		t.Fatalf("expected duplicate reliable message to be suppressed, got %#v", got)
	}
}

func TestReliableOrderedMessagesDispatchInOrderExternal(t *testing.T) {
	_, addr, recorder := startReliabilityHandlerServer(t)
	conn := dialUDPPacketSizeClient(t, addr)
	defer conn.Close()

	sendReliabilityClientMessage(t, conn, 1, 0, 0, 2, 22, network.DeliveryReliableOrdered)
	time.Sleep(20 * time.Millisecond)
	if got := recorder.Values(); len(got) != 0 {
		t.Fatalf("expected out-of-order reliable message to wait, got %#v", got)
	}

	sendReliabilityClientMessage(t, conn, 2, 0, 0, 1, 11, network.DeliveryReliableOrdered)
	waitForRecordedMessages(t, recorder, []uint64{11, 22})
}

func TestUnreliableMessagesStillBehaveAsBeforeExternal(t *testing.T) {
	_, addr, recorder := startReliabilityHandlerServer(t)
	conn := dialUDPPacketSizeClient(t, addr)
	defer conn.Close()

	sendReliabilityClientMessage(t, conn, 1, 0, 0, 0, 7, network.DeliveryUnreliable)
	sendReliabilityClientMessage(t, conn, 2, 0, 0, 0, 7, network.DeliveryUnreliable)

	waitForRecordedMessages(t, recorder, []uint64{7, 7})
}

func startReliabilityServer(t *testing.T, config network.Config) (*network.Server, *net.UDPAddr, *reliabilityObject) {
	t.Helper()

	object := &reliabilityObject{}
	server, addr := startUDPPacketSizeServer(t, config, object)
	return server, addr, object
}

func startReliabilityHandlerServer(t *testing.T) (*network.Server, *net.UDPAddr, *reliabilityRecorder) {
	t.Helper()

	recorder := &reliabilityRecorder{}
	port := freeUDPPacketSizePort(t)
	config := network.Config{
		UDPAddress:    fmt.Sprintf("127.0.0.1:%d", port),
		TickRate:      200,
		SnapshotRate:  1,
		ClientTimeout: time.Second,
	}
	server := network.NewServer(config)
	if err := server.RegisterWireMessage(network.WireMessageDefinition{
		ID:          reliabilityMessageID,
		Name:        "test.reliability",
		Direction:   network.WireDirectionClientToServer,
		MessageType: reflect.TypeOf(reliabilityClientMessage{}),
		Encode: func(message any) ([]byte, error) {
			return encodeReliabilityClientMessage(message.(reliabilityClientMessage)), nil
		},
		Decode: func(payload []byte) (any, error) {
			return decodeReliabilityClientMessage(payload)
		},
		Handler: func(ctx network.WireMessageContext) error {
			recorder.Append(ctx.Message.(reliabilityClientMessage).Number)
			return nil
		},
	}); err != nil {
		t.Fatalf("register reliability message: %v", err)
	}

	go func() {
		_ = server.ListenAndServeUDP()
	}()

	addr, err := net.ResolveUDPAddr("udp", config.UDPAddress)
	if err != nil {
		t.Fatalf("resolve UDP address: %v", err)
	}
	waitForUDPServerState(t, server)
	return server, addr, recorder
}

type reliabilityRecorder struct {
	mu     sync.Mutex
	values []uint64
}

func (recorder *reliabilityRecorder) Append(value uint64) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.values = append(recorder.values, value)
}

func (recorder *reliabilityRecorder) Values() []uint64 {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]uint64(nil), recorder.values...)
}

type reliableWireRead struct {
	packet  network.WirePacket
	message network.WireMessage
}

func readWireMessageType(t *testing.T, conn *net.UDPConn, messageType network.WireMessageType) reliableWireRead {
	t.Helper()

	buffer := make([]byte, 4096)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		bytesRead, err := conn.Read(buffer)
		if err != nil {
			if isUDPTimeout(err) {
				continue
			}
			t.Fatalf("read wire packet: %v", err)
		}

		packet, err := network.DecodePacket(buffer[:bytesRead])
		if err != nil {
			continue
		}
		for _, message := range packet.Messages {
			if message.Type == messageType {
				return reliableWireRead{packet: packet, message: message}
			}
		}
	}

	t.Fatalf("expected wire message 0x%04x", messageType)
	return reliableWireRead{}
}

func sendReliabilityObjectRequest(t *testing.T, conn *net.UDPConn, sequence, ack, ackBits uint64, action string) {
	t.Helper()

	message, err := network.EncodeClientMessage(network.ObjectRequest{
		ObjectType: "reliability",
		ObjectID:   "target",
		Action:     action,
		Data:       []byte("{}"),
	})
	if err != nil {
		t.Fatalf("encode object request: %v", err)
	}
	sendReliabilityPacket(t, conn, sequence, ack, ackBits, message)
}

func sendReliabilityPing(t *testing.T, conn *net.UDPConn, sequence, ack, ackBits uint64) {
	t.Helper()

	message, err := network.EncodeClientMessage(network.Ping{Nonce: sequence})
	if err != nil {
		t.Fatalf("encode ping: %v", err)
	}
	sendReliabilityPacket(t, conn, sequence, ack, ackBits, message)
}

func sendReliabilityClientMessage(t *testing.T, conn *net.UDPConn, sequence, ack, ackBits, deliveryID, number uint64, class network.DeliveryClass) {
	t.Helper()

	message := network.WireMessage{
		Type:       reliabilityMessageID,
		Payload:    encodeReliabilityClientMessage(reliabilityClientMessage{Number: number}),
		Delivery:   class,
		DeliveryID: deliveryID,
	}
	sendReliabilityPacket(t, conn, sequence, ack, ackBits, message)
}

func sendReliabilityPacket(t *testing.T, conn *net.UDPConn, sequence, ack, ackBits uint64, message network.WireMessage) {
	t.Helper()

	packet, err := network.EncodePacketWithAcks(sequence, ack, ackBits, []network.WireMessage{message})
	if err != nil {
		t.Fatalf("encode reliability packet: %v", err)
	}
	if _, err := conn.Write(packet); err != nil {
		t.Fatalf("send reliability packet: %v", err)
	}
}

func encodeReliabilityClientMessage(message reliabilityClientMessage) []byte {
	var payload [8]byte
	binary.BigEndian.PutUint64(payload[:], message.Number)
	return payload[:]
}

func decodeReliabilityClientMessage(payload []byte) (reliabilityClientMessage, error) {
	reader := bytes.NewReader(payload)
	var number uint64
	if err := binary.Read(reader, binary.BigEndian, &number); err != nil {
		return reliabilityClientMessage{}, err
	}
	if reader.Len() != 0 {
		return reliabilityClientMessage{}, fmt.Errorf("trailing reliability message bytes")
	}
	return reliabilityClientMessage{Number: number}, nil
}

func waitForReliableCounter(t *testing.T, server *network.Server, matches func(network.DebugUDPCounters) bool) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		counters := server.DebugState().UDP.Counters
		if matches(counters) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("reliable counters did not reach expected state: %#v", server.DebugState().UDP.Counters)
}

func waitForRecordedMessages(t *testing.T, recorder *reliabilityRecorder, expected []uint64) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if equalUint64s(recorder.Values(), expected) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected recorded messages %#v, got %#v", expected, recorder.Values())
}

func equalUint64s(left, right []uint64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
