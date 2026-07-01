package tests

import (
	"Enserva/network"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

type udpPacketSizeObject struct {
	id           string
	snapshotData string
	responseData string
}

func (object *udpPacketSizeObject) ObjectType() string {
	return "packet-size"
}

func (object *udpPacketSizeObject) ObjectID() string {
	return object.id
}

func (object *udpPacketSizeObject) Snapshot() any {
	return map[string]string{"value": object.snapshotData}
}

func (object *udpPacketSizeObject) OnRequest(ctx network.RequestContext) error {
	if object.responseData == "" {
		return nil
	}

	return ctx.Respond(network.ResponseMessage{
		Type:     "response",
		Sequence: ctx.Request.Sequence,
		OK:       true,
		Data:     map[string]string{"value": object.responseData},
	})
}

func TestConfigNormalizedHandlesInvalidMaxUDPPacketSize(t *testing.T) {
	defaulted := network.Config{MaxUDPPacketSize: 0}.Normalized()
	if defaulted.MaxUDPPacketSize != 1200 {
		t.Fatalf("expected zero max UDP packet size to default to 1200, got %d", defaulted.MaxUDPPacketSize)
	}

	negative := network.Config{MaxUDPPacketSize: -1}.Normalized()
	if negative.MaxUDPPacketSize != 1200 {
		t.Fatalf("expected negative max UDP packet size to default to 1200, got %d", negative.MaxUDPPacketSize)
	}

	configured := network.Config{MaxUDPPacketSize: 900}.Normalized()
	if configured.MaxUDPPacketSize != 900 {
		t.Fatalf("expected configured max UDP packet size 900, got %d", configured.MaxUDPPacketSize)
	}

	tooLarge := network.Config{MaxUDPPacketSize: 70000}.Normalized()
	if tooLarge.MaxUDPPacketSize != 65507 {
		t.Fatalf("expected too-large max UDP packet size to clamp to 65507, got %d", tooLarge.MaxUDPPacketSize)
	}
}

func TestMaxClientsDropsNewUDPClientsBeyondLimit(t *testing.T) {
	server, addr := startUDPPacketSizeServer(t, network.Config{
		TickRate:         200,
		SnapshotRate:     20,
		ClientTimeout:    time.Second,
		MaxClients:       1,
		MaxUDPPacketSize: 1200,
	}, &udpPacketSizeObject{
		id:           "max-clients",
		responseData: "ok",
	})

	first := dialUDPPacketSizeClient(t, addr)
	defer first.Close()
	_ = readUDPPacketSizePacket(t, first, udpPacketSizeRequest(t, 1, "max-clients"), 1200)

	second := dialUDPPacketSizeClient(t, addr)
	defer second.Close()
	buffer := make([]byte, 2048)
	deadline := time.Now().Add(750 * time.Millisecond)
	for sequence := uint64(1); time.Now().Before(deadline); sequence++ {
		if _, err := second.Write(udpPacketSizeRequest(t, sequence, "max-clients")); err != nil {
			t.Fatalf("send second client request: %v", err)
		}

		second.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
		bytesRead, err := second.Read(buffer)
		if err == nil {
			t.Fatalf("expected second client to be dropped, got %d byte packet: %s", bytesRead, string(buffer[:bytesRead]))
		}
		if !isUDPTimeout(err) {
			t.Fatalf("read second client response: %v", err)
		}

		state := server.DebugState().UDP
		if state.ClientCount == 1 && state.Counters.RequestsDropped > 0 {
			return
		}
	}

	t.Fatalf("expected max clients to keep one client and drop new clients, state: %#v", server.DebugState().UDP)
}

func TestOversizedSnapshotIsNotSent(t *testing.T) {
	server, addr := startUDPPacketSizeServer(t, network.Config{
		TickRate:         200,
		SnapshotRate:     200,
		MaxUDPPacketSize: 120,
	}, &udpPacketSizeObject{
		id:           "oversized-snapshot",
		snapshotData: strings.Repeat("x", 512),
	})

	conn := dialUDPPacketSizeClient(t, addr)
	defer conn.Close()

	request := udpPacketSizeRequest(t, 1, "oversized-snapshot")
	buffer := make([]byte, 2048)
	deadline := time.Now().Add(750 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := conn.Write(request); err != nil {
			t.Fatalf("send request: %v", err)
		}

		conn.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
		bytesRead, err := conn.Read(buffer)
		if err == nil {
			t.Fatalf("expected oversized snapshot to be dropped, got %d byte packet: %s", bytesRead, string(buffer[:bytesRead]))
		}
		if !isUDPTimeout(err) {
			t.Fatalf("read snapshot: %v", err)
		}

		if server.DebugState().UDP.Counters.OversizedOutbound > 0 {
			return
		}
	}

	t.Fatalf("expected oversized snapshot drop counter to increment")
}

func TestValidSnapshotUnderLimitSends(t *testing.T) {
	_, addr := startUDPPacketSizeServer(t, network.Config{
		TickRate:         200,
		SnapshotRate:     200,
		MaxUDPPacketSize: 1200,
	}, &udpPacketSizeObject{
		id:           "valid-snapshot",
		snapshotData: "ok",
	})

	conn := dialUDPPacketSizeClient(t, addr)
	defer conn.Close()

	request := udpPacketSizeRequest(t, 1, "valid-snapshot")
	packet := readUDPPacketSizePacket(t, conn, request, 1200)

	var snapshot network.SnapshotMessage
	if err := json.Unmarshal(packet, &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if snapshot.Type != "snapshot" {
		t.Fatalf("expected snapshot packet, got %#v", snapshot)
	}
	if _, ok := snapshot.Objects["packet-size"]["valid-snapshot"]; !ok {
		t.Fatalf("expected snapshot to include valid object: %#v", snapshot.Objects)
	}
}

func TestOversizedImmediateResponseIsDroppedOrReturnsError(t *testing.T) {
	server, addr := startUDPPacketSizeServer(t, network.Config{
		TickRate:         200,
		SnapshotRate:     20,
		MaxUDPPacketSize: 160,
	}, &udpPacketSizeObject{
		id:           "oversized-response",
		responseData: strings.Repeat("x", 512),
	})

	conn := dialUDPPacketSizeClient(t, addr)
	defer conn.Close()

	buffer := make([]byte, 2048)
	deadline := time.Now().Add(750 * time.Millisecond)
	for sequence := uint64(1); time.Now().Before(deadline); sequence++ {
		request := udpPacketSizeRequest(t, sequence, "oversized-response")
		if _, err := conn.Write(request); err != nil {
			t.Fatalf("send request: %v", err)
		}

		conn.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
		bytesRead, err := conn.Read(buffer)
		if err == nil {
			if bytesRead > 160 {
				t.Fatalf("received oversized response of %d bytes", bytesRead)
			}

			var response network.ResponseMessage
			if err := json.Unmarshal(buffer[:bytesRead], &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.OK || !strings.Contains(response.Error, network.ErrUDPPacketTooLarge.Error()) {
				t.Fatalf("expected predictable oversized-packet error response, got %#v", response)
			}
		} else if !isUDPTimeout(err) {
			t.Fatalf("read response: %v", err)
		}

		if server.DebugState().UDP.Counters.OversizedOutbound > 0 {
			return
		}
	}

	t.Fatalf("expected oversized immediate response drop counter to increment")
}

func TestValidImmediateResponseUnderLimitSends(t *testing.T) {
	_, addr := startUDPPacketSizeServer(t, network.Config{
		TickRate:         200,
		SnapshotRate:     20,
		MaxUDPPacketSize: 1200,
	}, &udpPacketSizeObject{
		id:           "valid-response",
		responseData: "ok",
	})

	conn := dialUDPPacketSizeClient(t, addr)
	defer conn.Close()

	request := udpPacketSizeRequest(t, 1, "valid-response")
	packet := readUDPPacketSizePacket(t, conn, request, 1200)

	var response network.ResponseMessage
	if err := json.Unmarshal(packet, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.OK || response.Type != "response" || response.Sequence != 1 {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func startUDPPacketSizeServer(t *testing.T, config network.Config, object network.Object) (*network.Server, *net.UDPAddr) {
	t.Helper()

	port := freeUDPPacketSizePort(t)
	config.UDPAddress = fmt.Sprintf("127.0.0.1:%d", port)
	if config.ClientTimeout == 0 {
		config.ClientTimeout = time.Second
	}

	server := network.NewServer(config)
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

func freeUDPPacketSizePort(t *testing.T) int {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("reserve UDP port: %v", err)
	}
	defer conn.Close()

	return conn.LocalAddr().(*net.UDPAddr).Port
}

func waitForUDPServerState(t *testing.T, server *network.Server) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if server.UDPServer() != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("udp server did not start")
}

func dialUDPPacketSizeClient(t *testing.T, addr *net.UDPAddr) *net.UDPConn {
	t.Helper()

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatalf("dial UDP server: %v", err)
	}
	return conn
}

func readUDPPacketSizePacket(t *testing.T, conn *net.UDPConn, request []byte, limit int) []byte {
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
			t.Fatalf("received %d byte packet over limit %d", bytesRead, limit)
		}

		packet := append([]byte(nil), buffer[:bytesRead]...)
		if isUDPPacketSizeErrorResponse(packet) {
			continue
		}
		return packet
	}

	t.Fatalf("expected UDP packet under limit")
	return nil
}

func isUDPPacketSizeErrorResponse(packet []byte) bool {
	var response network.ResponseMessage
	if err := json.Unmarshal(packet, &response); err != nil {
		return false
	}
	return response.Type == "error"
}

func isUDPTimeout(err error) bool {
	netErr, ok := err.(net.Error)
	return ok && netErr.Timeout()
}

func udpPacketSizeRequest(t *testing.T, sequence uint64, objectID string) []byte {
	t.Helper()

	payload, err := json.Marshal(network.RequestMessage{
		Sequence:   sequence,
		ObjectType: "packet-size",
		ObjectID:   objectID,
		Action:     "respond",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return payload
}
