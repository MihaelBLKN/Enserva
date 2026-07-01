package tests

import (
	"Enserva/network"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"
)

type deltaSnapshotTestObject struct {
	id    string
	value int
}

func (object *deltaSnapshotTestObject) ObjectType() string {
	return "delta"
}

func (object *deltaSnapshotTestObject) ObjectID() string {
	return object.id
}

func (object *deltaSnapshotTestObject) Snapshot() any {
	return map[string]any{
		"id":    object.id,
		"value": object.value,
	}
}

func TestBuildSnapshotDeltaTracksObjectLifecycle(t *testing.T) {
	previous := network.SnapshotData{
		"building": {
			"changed":   map[string]any{"hp": 10},
			"despawned": map[string]any{"hp": 1},
			"same":      map[string]any{"hp": 7},
		},
	}
	current := network.SnapshotData{
		"building": {
			"changed": map[string]any{"hp": 8},
			"same":    map[string]any{"hp": 7},
			"spawned": map[string]any{"hp": 3},
		},
	}

	delta := network.BuildSnapshotDelta(previous, current)
	if hasSnapshotObject(delta.Changed, "building", "same") {
		t.Fatalf("expected unchanged object to be omitted: %#v", delta)
	}
	if !hasSnapshotObject(delta.Changed, "building", "changed") {
		t.Fatalf("expected changed object in delta: %#v", delta)
	}
	if !hasSnapshotObject(delta.Spawned, "building", "spawned") {
		t.Fatalf("expected new object in spawned delta: %#v", delta)
	}
	if len(delta.Despawned) != 1 || delta.Despawned[0].ObjectType != "building" || delta.Despawned[0].ObjectID != "despawned" {
		t.Fatalf("expected despawned object in delta: %#v", delta)
	}
}

func TestSnapshotDeltaRespectsSceneAndInterestFiltering(t *testing.T) {
	runtime := network.NewRuntime(network.Config{})
	player := &interestTestObject{
		objectType:  "player",
		id:          "player-1",
		subject:     network.InterestPlayer,
		radius:      10,
		includeSelf: true,
	}
	near := &interestTestObject{objectType: "building", id: "near", subject: network.InterestGameObject, x: 2}
	changing := &interestTestObject{objectType: "building", id: "changing", subject: network.InterestGameObject, x: 4}
	far := &interestTestObject{objectType: "building", id: "far", subject: network.InterestGameObject, x: 100}
	otherScene := &interestTestObject{objectType: "building", id: "other-scene", subject: network.InterestGameObject, x: 1}

	mustRegisterInterestObject(t, runtime, player)
	mustRegisterInterestObject(t, runtime, near)
	mustRegisterInterestObject(t, runtime, changing)
	mustRegisterInterestObject(t, runtime, far)
	mustRegisterInterestObject(t, runtime, otherScene)

	features := runtime.Features()
	features.SetClientScene("player-1", "map-1")
	features.SetObjectScene("player", "player-1", "map-1")
	features.SetObjectScene("building", "near", "map-1")
	features.SetObjectScene("building", "changing", "map-1")
	features.SetObjectScene("building", "far", "map-1")
	features.SetObjectScene("building", "other-scene", "map-2")

	previous := runtime.SnapshotForClient("player-1")
	near.x = 100
	changing.x = 5
	far.x = 5
	otherScene.x = 2
	current := runtime.SnapshotForClient("player-1")
	delta := network.BuildSnapshotDelta(previous, current)

	if !hasSnapshotObject(delta.Changed, "building", "changing") {
		t.Fatalf("expected changed visible object in delta: %#v", delta)
	}
	if !hasSnapshotObject(delta.Spawned, "building", "far") {
		t.Fatalf("expected newly visible object in spawned delta: %#v", delta)
	}
	if !hasDespawnedObject(delta.Despawned, "building", "near") {
		t.Fatalf("expected no-longer-visible object in despawned delta: %#v", delta)
	}
	if hasSnapshotObject(delta.Changed, "building", "other-scene") || hasSnapshotObject(delta.Spawned, "building", "other-scene") {
		t.Fatalf("expected invisible other-scene object to stay out of delta: %#v", delta)
	}
}

func TestDeltaSnapshotServerFirstSnapshotAndFullInterval(t *testing.T) {
	server, addr := startUDPPacketSizeServer(t, network.Config{
		TickRate:             200,
		SnapshotRate:         200,
		EnableDeltaSnapshots: true,
		FullSnapshotInterval: 2,
		MaxUDPPacketSize:     1200,
	}, &deltaSnapshotTestObject{id: "target", value: 1})

	conn := dialUDPPacketSizeClient(t, addr)
	defer conn.Close()

	if _, err := conn.Write(deltaSnapshotRequest(t, 1, "target")); err != nil {
		t.Fatalf("send request: %v", err)
	}

	first := readDeltaSnapshotJSON(t, conn)
	second := readDeltaSnapshotJSONType(t, conn, "snapshot.delta")
	third := readDeltaSnapshotJSONType(t, conn, "snapshot")

	if first.Type != "snapshot" {
		t.Fatalf("expected first snapshot to be full, got %#v", first)
	}
	if second.Type != "snapshot.delta" {
		t.Fatalf("expected second snapshot to be delta, got %#v", second)
	}
	if _, ok := second.Changed["delta"]["target"]; ok {
		t.Fatalf("expected unchanged object to be omitted from delta: %#v", second)
	}
	if third.Type != "snapshot" {
		t.Fatalf("expected full snapshot interval to force a full snapshot, got %#v", third)
	}

	counters := server.DebugState().UDP.Counters
	if counters.FullSnapshotsSent < 2 || counters.DeltaSnapshotsSent < 1 {
		t.Fatalf("expected full and delta counters to increment, got %#v", counters)
	}
}

func TestNegotiatedWireClientWithoutDeltaCapabilityReceivesFullSnapshots(t *testing.T) {
	server, addr := startDeltaSnapshotAuthServer(t, network.Config{
		TickRate:             200,
		SnapshotRate:         200,
		EnableDeltaSnapshots: true,
		FullSnapshotInterval: 10,
		MaxUDPPacketSize:     1200,
	}, &deltaSnapshotTestObject{id: "target", value: 1})

	conn := dialUDPPacketSizeClient(t, addr)
	defer conn.Close()

	hello, err := network.EncodeClientMessage(network.ClientHello{
		ClientName:      "wire-client",
		Token:           "valid-token",
		ProtocolVersion: network.WireProtocolVersion,
		Capabilities:    network.WireCapabilityReliableOrdered | network.WireCapabilityReliableUnordered,
		MaxPacketSize:   1200,
	})
	if err != nil {
		t.Fatalf("encode hello: %v", err)
	}
	packet, err := network.EncodePacket(1, []network.WireMessage{hello})
	if err != nil {
		t.Fatalf("encode hello packet: %v", err)
	}
	if _, err := conn.Write(packet); err != nil {
		t.Fatalf("send hello: %v", err)
	}

	first := readWireSnapshotMessageType(t, conn)
	second := readWireSnapshotMessageType(t, conn)

	if first != network.WireMessageWorldSnapshot {
		t.Fatalf("expected first snapshot to be full, got 0x%04x", first)
	}
	if second != network.WireMessageWorldSnapshot {
		t.Fatalf("expected negotiated client without delta capability to receive full snapshot, got 0x%04x", second)
	}

	counters := server.DebugState().UDP.Counters
	if counters.DeltaSnapshotsSent != 0 {
		t.Fatalf("expected no delta snapshots for client without delta capability, got %#v", counters)
	}
}

type deltaSnapshotJSON struct {
	Type    string               `json:"type"`
	Changed network.SnapshotData `json:"changed"`
}

func readDeltaSnapshotJSON(t *testing.T, conn *net.UDPConn) deltaSnapshotJSON {
	t.Helper()

	buffer := make([]byte, 2048)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		bytesRead, err := conn.Read(buffer)
		if err != nil {
			if isUDPTimeout(err) {
				continue
			}
			t.Fatalf("read delta snapshot: %v", err)
		}

		var snapshot deltaSnapshotJSON
		if err := json.Unmarshal(buffer[:bytesRead], &snapshot); err != nil {
			t.Fatalf("decode delta snapshot: %v", err)
		}
		if snapshot.Type == "error" {
			continue
		}
		if snapshot.Changed == nil {
			snapshot.Changed = network.SnapshotData{}
		}
		return snapshot
	}

	t.Fatalf("expected delta snapshot packet")
	return deltaSnapshotJSON{}
}

func readDeltaSnapshotJSONType(t *testing.T, conn *net.UDPConn, snapshotType string) deltaSnapshotJSON {
	t.Helper()

	buffer := make([]byte, 2048)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		bytesRead, err := conn.Read(buffer)
		if err != nil {
			if isUDPTimeout(err) {
				continue
			}
			t.Fatalf("read delta snapshot: %v", err)
		}

		var snapshot deltaSnapshotJSON
		if err := json.Unmarshal(buffer[:bytesRead], &snapshot); err != nil {
			t.Fatalf("decode delta snapshot: %v", err)
		}
		if snapshot.Type != snapshotType {
			continue
		}
		if snapshot.Changed == nil {
			snapshot.Changed = network.SnapshotData{}
		}
		return snapshot
	}

	t.Fatalf("expected %s packet", snapshotType)
	return deltaSnapshotJSON{}
}

func deltaSnapshotRequest(t *testing.T, sequence uint64, objectID string) []byte {
	t.Helper()

	payload, err := json.Marshal(network.RequestMessage{
		Sequence:   sequence,
		ObjectType: "delta",
		ObjectID:   objectID,
		Action:     fmt.Sprintf("touch-%d", sequence),
	})
	if err != nil {
		t.Fatalf("marshal delta request: %v", err)
	}
	return payload
}

func startDeltaSnapshotAuthServer(t *testing.T, config network.Config, object network.Object) (*network.Server, *net.UDPAddr) {
	t.Helper()

	port := freeUDPPacketSizePort(t)
	config.UDPAddress = fmt.Sprintf("127.0.0.1:%d", port)
	if config.ClientTimeout == 0 {
		config.ClientTimeout = time.Second
	}

	server := network.NewServer(config)
	if err := server.RegisterAuthenticationObject(&authenticationTestObject{id: "auth", returnID: "delta-client"}); err != nil {
		t.Fatalf("register auth object: %v", err)
	}
	if err := server.RegisterObject(object); err != nil {
		t.Fatalf("register object: %v", err)
	}

	go func() {
		_ = server.ListenAndServeUDP()
	}()

	addr, err := net.ResolveUDPAddr("udp", config.UDPAddress)
	if err != nil {
		t.Fatalf("resolve UDP address: %v", err)
	}
	waitForUDPServerState(t, server)
	return server, addr
}

func readWireSnapshotMessageType(t *testing.T, conn *net.UDPConn) network.WireMessageType {
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
			t.Fatalf("read wire snapshot: %v", err)
		}

		packet, err := network.DecodePacket(buffer[:bytesRead])
		if err != nil {
			continue
		}
		for _, message := range packet.Messages {
			if message.Type == network.WireMessageWorldSnapshot || message.Type == network.WireMessageDeltaSnapshot {
				return message.Type
			}
		}
	}

	t.Fatalf("expected wire snapshot packet")
	return 0
}

func hasDespawnedObject(objects []network.SnapshotObjectRef, objectType, objectID string) bool {
	for _, object := range objects {
		if object.ObjectType == objectType && object.ObjectID == objectID {
			return true
		}
	}
	return false
}
