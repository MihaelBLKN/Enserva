package tests

import (
	"Enserva/network"
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"
)

type customMoveMessage struct {
	EntityID string
	X        float32
}

func TestWirePacketRoundTripWithPlayerInput(t *testing.T) {
	message, err := network.EncodeClientMessage(network.PlayerInput{
		ObjectID: "player-1",
		X:        1,
		Y:        -0.5,
		Z:        0.25,
	})
	if err != nil {
		t.Fatalf("encode player input: %v", err)
	}

	payload, err := network.EncodePacket(42, []network.WireMessage{message})
	if err != nil {
		t.Fatalf("encode packet: %v", err)
	}

	packet, err := network.DecodePacket(payload)
	if err != nil {
		t.Fatalf("decode packet: %v", err)
	}
	if packet.Version != network.WireProtocolVersion {
		t.Fatalf("expected version %d, got %d", network.WireProtocolVersion, packet.Version)
	}
	if packet.Sequence != 42 {
		t.Fatalf("expected sequence 42, got %d", packet.Sequence)
	}
	if len(packet.Messages) != 1 {
		t.Fatalf("expected one message, got %d", len(packet.Messages))
	}

	decoded, err := network.DecodeClientMessage(packet.Messages[0])
	if err != nil {
		t.Fatalf("decode client message: %v", err)
	}
	input, ok := decoded.(network.PlayerInput)
	if !ok {
		t.Fatalf("expected PlayerInput, got %T", decoded)
	}
	if input.ObjectID != "player-1" || input.X != 1 || input.Y != -0.5 || input.Z != 0.25 {
		t.Fatalf("unexpected decoded input: %#v", input)
	}
}

func TestWirePacketRoundTripWithGenericClientInput(t *testing.T) {
	message, err := network.EncodeClientMessage(network.GenericClientInput{
		Sequence:   12,
		Tick:       64,
		ObjectType: "ship",
		ObjectID:   "ship-1",
		TargetID:   "engine",
		Payload:    []byte{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("encode generic client input: %v", err)
	}
	if message.Type != network.WireMessageClientInput {
		t.Fatalf("expected client input message id, got 0x%04x", message.Type)
	}

	payload, err := network.EncodePacket(42, []network.WireMessage{message})
	if err != nil {
		t.Fatalf("encode packet: %v", err)
	}
	packet, err := network.DecodePacket(payload)
	if err != nil {
		t.Fatalf("decode packet: %v", err)
	}
	decoded, err := network.DecodeClientMessage(packet.Messages[0])
	if err != nil {
		t.Fatalf("decode client message: %v", err)
	}
	input, ok := decoded.(network.GenericClientInput)
	if !ok {
		t.Fatalf("expected GenericClientInput, got %T", decoded)
	}
	if input.Sequence != 12 || input.Tick != 64 || input.ObjectType != "ship" || input.ObjectID != "ship-1" || input.TargetID != "engine" || !reflect.DeepEqual(input.Payload, []byte{1, 2, 3}) {
		t.Fatalf("unexpected decoded input: %#v", input)
	}
}

func TestWirePacketRoundTripWithAcks(t *testing.T) {
	message, err := network.EncodeClientMessage(network.Ping{Nonce: 99})
	if err != nil {
		t.Fatalf("encode ping: %v", err)
	}
	payload, err := network.EncodePacketWithAcks(100, 91, 0b1011, []network.WireMessage{message})
	if err != nil {
		t.Fatalf("encode packet with acks: %v", err)
	}

	packet, err := network.DecodePacket(payload)
	if err != nil {
		t.Fatalf("decode packet: %v", err)
	}
	if packet.Sequence != 100 || packet.Ack != 91 || packet.AckBits != 0b1011 {
		t.Fatalf("unexpected packet ack state: %#v", packet)
	}
}

func TestWireWorldSnapshotRoundTrip(t *testing.T) {
	message, err := network.EncodeServerMessage(network.WorldSnapshot{
		ClientID:     "player-1",
		Tick:         128,
		LastSequence: 42,
		Objects: network.SnapshotData{
			"player": {
				"player-1": map[string]any{
					"id":        "player-1",
					"x":         12.5,
					"requests":  uint64(7),
					"connected": true,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("encode snapshot: %v", err)
	}

	packetBytes, err := network.EncodePacket(128, []network.WireMessage{message})
	if err != nil {
		t.Fatalf("encode packet: %v", err)
	}
	packet, err := network.DecodePacket(packetBytes)
	if err != nil {
		t.Fatalf("decode packet: %v", err)
	}
	decoded, err := network.DecodeServerMessage(packet.Messages[0])
	if err != nil {
		t.Fatalf("decode server message: %v", err)
	}

	snapshot, ok := decoded.(network.WorldSnapshot)
	if !ok {
		t.Fatalf("expected WorldSnapshot, got %T", decoded)
	}
	if snapshot.ClientID != "player-1" || snapshot.Tick != 128 || snapshot.LastSequence != 42 {
		t.Fatalf("unexpected snapshot header: %#v", snapshot)
	}
	player := snapshot.Objects["player"]["player-1"].(map[string]any)
	if player["id"] != "player-1" || player["x"] != 12.5 || player["requests"] != uint64(7) || player["connected"] != true {
		t.Fatalf("unexpected snapshot payload: %#v", player)
	}
}

func TestWireWorldDeltaSnapshotRoundTrip(t *testing.T) {
	message, err := network.EncodeServerMessage(network.WorldDeltaSnapshot{
		ClientID:     "player-1",
		Tick:         130,
		LastSequence: 45,
		BaselineTick: 128,
		Spawned: network.SnapshotData{
			"pickup": {
				"coin-1": map[string]any{"value": uint64(5)},
			},
		},
		Changed: network.SnapshotData{
			"player": {
				"player-1": map[string]any{"x": 15.5},
			},
		},
		Despawned: []network.SnapshotObjectRef{{
			ObjectType: "projectile",
			ObjectID:   "bolt-1",
		}},
	})
	if err != nil {
		t.Fatalf("encode delta snapshot: %v", err)
	}

	packetBytes, err := network.EncodePacket(130, []network.WireMessage{message})
	if err != nil {
		t.Fatalf("encode packet: %v", err)
	}
	packet, err := network.DecodePacket(packetBytes)
	if err != nil {
		t.Fatalf("decode packet: %v", err)
	}
	decoded, err := network.DecodeServerMessage(packet.Messages[0])
	if err != nil {
		t.Fatalf("decode server message: %v", err)
	}

	snapshot, ok := decoded.(network.WorldDeltaSnapshot)
	if !ok {
		t.Fatalf("expected WorldDeltaSnapshot, got %T", decoded)
	}
	if snapshot.ClientID != "player-1" || snapshot.Tick != 130 || snapshot.LastSequence != 45 || snapshot.BaselineTick != 128 {
		t.Fatalf("unexpected delta snapshot header: %#v", snapshot)
	}
	changed := snapshot.Changed["player"]["player-1"].(map[string]any)
	if changed["x"] != 15.5 {
		t.Fatalf("unexpected changed payload: %#v", changed)
	}
	spawned := snapshot.Spawned["pickup"]["coin-1"].(map[string]any)
	if spawned["value"] != uint64(5) {
		t.Fatalf("unexpected spawned payload: %#v", spawned)
	}
	if len(snapshot.Despawned) != 1 || snapshot.Despawned[0].ObjectType != "projectile" || snapshot.Despawned[0].ObjectID != "bolt-1" {
		t.Fatalf("unexpected despawned payload: %#v", snapshot.Despawned)
	}
}

func TestWireRejectsInvalidPacketData(t *testing.T) {
	if _, err := network.DecodePacket([]byte{0x45}); !errors.Is(err, network.ErrInvalidWirePacket) {
		t.Fatalf("expected invalid packet error, got %v", err)
	}

	message, err := network.EncodeClientMessage(network.Ping{Nonce: 9})
	if err != nil {
		t.Fatalf("encode ping: %v", err)
	}
	packet, err := network.EncodePacket(1, []network.WireMessage{message})
	if err != nil {
		t.Fatalf("encode packet: %v", err)
	}
	packet[len(packet)-1] = 0
	truncated := packet[:len(packet)-1]
	if _, err := network.DecodePacket(truncated); !errors.Is(err, network.ErrInvalidWirePacket) {
		t.Fatalf("expected invalid truncated packet error, got %v", err)
	}
}

func TestWireRejectsVersionMismatch(t *testing.T) {
	message, err := network.EncodeClientMessage(network.Ping{Nonce: 1})
	if err != nil {
		t.Fatalf("encode ping: %v", err)
	}
	packet, err := network.EncodePacket(1, []network.WireMessage{message})
	if err != nil {
		t.Fatalf("encode packet: %v", err)
	}
	packet[2] = network.WireProtocolVersion + 1

	if _, err := network.DecodePacket(packet); !errors.Is(err, network.ErrUnsupportedWireVersion) {
		t.Fatalf("expected version error, got %v", err)
	}
}

func TestWireRejectsOversizedMessage(t *testing.T) {
	oversized := make([]byte, network.MaxWireMessagePayloadSize+1)
	_, err := network.EncodePacket(1, []network.WireMessage{{
		Type:    network.WireMessageGameMin,
		Payload: oversized,
	}})
	if !errors.Is(err, network.ErrWireMessageTooLarge) {
		t.Fatalf("expected oversized message error, got %v", err)
	}
}

func TestWireUnknownMessageTypesAreSkippable(t *testing.T) {
	packetBytes, err := network.EncodePacket(7, []network.WireMessage{{
		Type:    network.WireMessageType(250),
		Payload: []byte{1, 2, 3},
	}})
	if err != nil {
		t.Fatalf("encode unknown packet: %v", err)
	}
	packet, err := network.DecodePacket(packetBytes)
	if err != nil {
		t.Fatalf("decode packet: %v", err)
	}
	decoded, err := network.DecodeClientMessage(packet.Messages[0])
	if err != nil {
		t.Fatalf("decode unknown message: %v", err)
	}
	unknown, ok := decoded.(network.UnknownWireMessage)
	if !ok {
		t.Fatalf("expected unknown message, got %T", decoded)
	}
	if unknown.Type != network.WireMessageType(250) || len(unknown.Payload) != 3 {
		t.Fatalf("unexpected unknown message: %#v", unknown)
	}
}

func TestCustomWireMessageRegistrationEncodeDecodeAndDispatch(t *testing.T) {
	runtime := network.NewRuntime(network.Config{})
	handled := false

	err := runtime.RegisterWireMessage(network.WireMessageDefinition{
		ID:          network.WireMessageGameMin + 1,
		Name:        "game.custom_move",
		Direction:   network.WireDirectionClientToServer,
		MessageType: reflect.TypeOf(customMoveMessage{}),
		Encode: func(message any) ([]byte, error) {
			move := message.(customMoveMessage)
			buffer := bytes.Buffer{}
			binary.Write(&buffer, binary.BigEndian, uint16(len(move.EntityID)))
			buffer.WriteString(move.EntityID)
			binary.Write(&buffer, binary.BigEndian, move.X)
			return buffer.Bytes(), nil
		},
		Decode: func(payload []byte) (any, error) {
			reader := bytes.NewReader(payload)
			var length uint16
			if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
				return nil, err
			}
			entityID := make([]byte, length)
			if _, err := reader.Read(entityID); err != nil {
				return nil, err
			}
			var x float32
			if err := binary.Read(reader, binary.BigEndian, &x); err != nil {
				return nil, err
			}
			return customMoveMessage{EntityID: string(entityID), X: x}, nil
		},
		Validate: func(message any) error {
			if message.(customMoveMessage).EntityID == "" {
				return errors.New("missing entity id")
			}
			return nil
		},
		Handler: func(ctx network.WireMessageContext) error {
			move := ctx.Message.(customMoveMessage)
			if ctx.Runtime != runtime || move.EntityID != "ship-1" || move.X != 3.5 {
				t.Fatalf("unexpected handler context: %#v %#v", ctx, move)
			}
			handled = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("register custom message: %v", err)
	}

	message, err := runtime.WireMessages().EncodeMessage(customMoveMessage{EntityID: "ship-1", X: 3.5})
	if err != nil {
		t.Fatalf("encode custom message: %v", err)
	}
	if message.Type != network.WireMessageGameMin+1 {
		t.Fatalf("unexpected custom message id: 0x%04x", message.Type)
	}

	decoded, err := runtime.WireMessages().DecodeMessage(message)
	if err != nil {
		t.Fatalf("decode custom message: %v", err)
	}
	handledByRegistry, err := runtime.WireMessages().Dispatch(network.WireMessageContext{
		Transport: "test",
		MessageID: message.Type,
		Message:   decoded,
		Runtime:   runtime,
	})
	if err != nil {
		t.Fatalf("dispatch custom message: %v", err)
	}
	if !handledByRegistry || !handled {
		t.Fatalf("expected custom handler to run")
	}
}

func TestCustomWireMessageValidationFailure(t *testing.T) {
	runtime := network.NewRuntime(network.Config{})
	err := runtime.RegisterWireMessage(network.WireMessageDefinition{
		ID:          network.WireMessageGameMin + 2,
		Name:        "game.invalid_move",
		Direction:   network.WireDirectionClientToServer,
		MessageType: reflect.TypeOf(customMoveMessage{}),
		Encode:      func(message any) ([]byte, error) { return nil, nil },
		Decode:      func(payload []byte) (any, error) { return customMoveMessage{}, nil },
		Validate: func(message any) error {
			return errors.New("invalid")
		},
	})
	if err != nil {
		t.Fatalf("register custom message: %v", err)
	}

	_, err = runtime.WireMessages().EncodeMessage(customMoveMessage{EntityID: "ship-1"})
	if !errors.Is(err, network.ErrWireMessageValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestTypedPayloadDecodeAvoidsRequestJSON(t *testing.T) {
	var request struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
		Z float64 `json:"z"`
	}
	ctx := network.RequestContext{
		Payload: network.PlayerInput{
			ObjectID: "player-1",
			X:        1,
			Y:        -1,
			Z:        0.5,
		},
	}

	if err := ctx.Decode(&request); err != nil {
		t.Fatalf("decode typed payload: %v", err)
	}
	if request.X != 1 || request.Y != -1 || request.Z != 0.5 {
		t.Fatalf("unexpected decoded request: %#v", request)
	}
}
