package tests

import (
	"Enserva/network"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

type budgetTestObject struct {
	objectType string
	objectID   string
	priority   network.OutboundPriority
	payload    string
}

func (object budgetTestObject) ObjectType() string {
	return object.objectType
}

func (object budgetTestObject) ObjectID() string {
	return object.objectID
}

func (object budgetTestObject) Snapshot() any {
	return map[string]any{
		"id":      object.objectID,
		"payload": object.payload,
	}
}

func (object budgetTestObject) SnapshotPriority() network.OutboundPriority {
	return object.priority
}

func TestDisabledBudgetPreservesOutboundTraffic(t *testing.T) {
	_, addr := startUDPPacketSizeServer(t, network.Config{
		TickRate:              200,
		SnapshotRate:          200,
		EnableBandwidthBudget: false,
		ClientBytesPerSecond:  1,
		MaxUDPPacketSize:      1200,
	}, &udpPacketSizeObject{
		id:           "disabled-budget",
		snapshotData: "ok",
	})

	conn := dialUDPPacketSizeClient(t, addr)
	defer conn.Close()

	packet := readUDPPacketSizePacket(t, conn, udpPacketSizeRequest(t, 1, "disabled-budget"), 1200)
	var snapshot network.SnapshotMessage
	if err := json.Unmarshal(packet, &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if _, ok := snapshot.Objects["packet-size"]["disabled-budget"]; !ok {
		t.Fatalf("expected disabled budgeting to allow snapshot traffic: %#v", snapshot.Objects)
	}
}

func TestBandwidthBudgetDropsImmediateTrafficWhenExhausted(t *testing.T) {
	server, addr := startUDPPacketSizeServer(t, network.Config{
		TickRate:              200,
		SnapshotRate:          1,
		EnableBandwidthBudget: true,
		ClientBytesPerSecond:  1,
		MaxUDPPacketSize:      1200,
	}, &udpPacketSizeObject{
		id:           "budget-deferred",
		snapshotData: strings.Repeat("x", 64),
		responseData: "ok",
	})

	conn := dialUDPPacketSizeClient(t, addr)
	defer conn.Close()

	request := udpPacketSizeRequest(t, 1, "budget-deferred")
	buffer := make([]byte, 2048)
	deadline := time.Now().Add(750 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := conn.Write(request); err != nil {
			t.Fatalf("send request: %v", err)
		}

		conn.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
		bytesRead, err := conn.Read(buffer)
		if err == nil {
			t.Fatalf("expected budgeted traffic to be deferred or dropped, got %d byte packet: %s", bytesRead, string(buffer[:bytesRead]))
		}
		if !isUDPTimeout(err) {
			t.Fatalf("read packet: %v", err)
		}

		if server.DebugState().UDP.Counters.BudgetDrops > 0 {
			return
		}
	}

	t.Fatalf("expected bandwidth budget drop counter to increment")
}

func TestSnapshotBudgetKeepsHighPriorityBeforeLowPriority(t *testing.T) {
	_, warmupAddr := startUDPPacketSizeServerWithObjects(t, network.Config{
		TickRate:                200,
		SnapshotRate:            200,
		EnableBandwidthBudget:   true,
		ClientBytesPerSecond:    10_000,
		DefaultSnapshotPriority: network.OutboundPriorityNormal,
		MaxUDPPacketSize:        1200,
	}, []network.Object{
		budgetTestObject{
			objectType: "budget",
			objectID:   "high",
			priority:   network.OutboundPriorityHigh,
			payload:    "important",
		},
		budgetTestObject{
			objectType: "budget",
			objectID:   "low",
			priority:   network.OutboundPriorityLow,
			payload:    strings.Repeat("low-priority-payload-that-can-wait", 6),
		},
	})

	conn := dialUDPPacketSizeClient(t, warmupAddr)
	defer conn.Close()

	if _, err := conn.Write(udpBudgetRequest(t, 1)); err != nil {
		t.Fatalf("send request: %v", err)
	}

	var highOnlyLen int
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
		packet := make([]byte, 2048)
		bytesRead, err := conn.Read(packet)
		if err != nil {
			if isUDPTimeout(err) {
				continue
			}
			t.Fatalf("read warmup snapshot: %v", err)
		}

		var snapshot network.SnapshotMessage
		if err := json.Unmarshal(packet[:bytesRead], &snapshot); err != nil {
			continue
		}
		if hasBudgetSnapshotObject(snapshot.Objects, "budget", "high") && hasBudgetSnapshotObject(snapshot.Objects, "budget", "low") {
			highOnlyLen = len(mustMarshalBudgetSnapshot(t, snapshot, "high"))
			break
		}
	}
	if highOnlyLen == 0 {
		t.Fatalf("expected warmup snapshot with high and low objects")
	}

	_, addr := startUDPPacketSizeServerWithObjects(t, network.Config{
		TickRate:                200,
		SnapshotRate:            200,
		EnableBandwidthBudget:   true,
		ClientBytesPerSecond:    highOnlyLen + 16,
		DefaultSnapshotPriority: network.OutboundPriorityNormal,
		MaxUDPPacketSize:        1200,
	}, []network.Object{
		budgetTestObject{
			objectType: "budget",
			objectID:   "high",
			priority:   network.OutboundPriorityHigh,
			payload:    "important",
		},
		budgetTestObject{
			objectType: "budget",
			objectID:   "low",
			priority:   network.OutboundPriorityLow,
			payload:    strings.Repeat("low-priority-payload-that-can-wait", 6),
		},
	})

	conn2 := dialUDPPacketSizeClient(t, addr)
	defer conn2.Close()
	packet := readBudgetSnapshotPacket(t, conn2, udpBudgetRequest(t, 1), highOnlyLen+48)

	var snapshot network.SnapshotMessage
	if err := json.Unmarshal(packet, &snapshot); err != nil {
		t.Fatalf("decode filtered snapshot: %v", err)
	}
	if !hasBudgetSnapshotObject(snapshot.Objects, "budget", "high") {
		t.Fatalf("expected high-priority object to remain: %#v", snapshot.Objects)
	}
	if hasBudgetSnapshotObject(snapshot.Objects, "budget", "low") {
		t.Fatalf("expected low-priority object to be deferred first: %#v", snapshot.Objects)
	}
}

func hasBudgetSnapshotObject(snapshot network.SnapshotData, objectType, objectID string) bool {
	objectsByID := snapshot[objectType]
	if objectsByID == nil {
		return false
	}
	_, ok := objectsByID[objectID]
	return ok
}

func startUDPPacketSizeServerWithObjects(t *testing.T, config network.Config, objects []network.Object) (*network.Server, *net.UDPAddr) {
	t.Helper()

	server, addr := startUDPPacketSizeServer(t, config, objects[0])
	for _, object := range objects[1:] {
		if err := server.RegisterObject(object); err != nil {
			t.Fatalf("register object: %v", err)
		}
	}
	return server, addr
}

func udpBudgetRequest(t *testing.T, sequence uint64) []byte {
	t.Helper()

	payload, err := json.Marshal(network.RequestMessage{
		Sequence:   sequence,
		ObjectType: "budget",
		ObjectID:   "high",
		Action:     "noop",
	})
	if err != nil {
		t.Fatalf("marshal budget request: %v", err)
	}
	return payload
}

func mustMarshalBudgetSnapshot(t *testing.T, snapshot network.SnapshotMessage, keepObjectID string) []byte {
	t.Helper()

	snapshot.Objects = network.SnapshotData{
		"budget": {
			keepObjectID: snapshot.Objects["budget"][keepObjectID],
		},
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal high-only snapshot: %v", err)
	}
	return payload
}

func readBudgetSnapshotPacket(t *testing.T, conn *net.UDPConn, request []byte, limit int) []byte {
	t.Helper()

	buffer := make([]byte, 2048)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := conn.Write(request); err != nil {
			t.Fatalf("send request: %v", err)
		}

		conn.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
		bytesRead, err := conn.Read(buffer)
		if err != nil {
			if isUDPTimeout(err) {
				continue
			}
			t.Fatalf("read UDP packet: %v", err)
		}
		if bytesRead > limit {
			continue
		}
		packet := append([]byte(nil), buffer[:bytesRead]...)
		var snapshot network.SnapshotMessage
		if err := json.Unmarshal(packet, &snapshot); err != nil || snapshot.Type != "snapshot" {
			continue
		}
		return packet
	}

	t.Fatalf("expected budgeted UDP snapshot under limit")
	return nil
}
