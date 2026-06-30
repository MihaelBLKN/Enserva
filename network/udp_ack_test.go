package network

import (
	"net"
	"testing"
)

func TestUDPClientAckBitsTrackReceivedSequences(t *testing.T) {
	server := NewUDPServer(NewRuntime(Config{}))
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:45678")
	if err != nil {
		t.Fatalf("resolve addr: %v", err)
	}

	client, accepted := server.acceptClientRequest(addr, 10, 0, 0)
	if !accepted {
		t.Fatalf("expected sequence 10 to be accepted")
	}
	if _, accepted = server.acceptClientRequest(addr, 8, 0, 0); !accepted {
		t.Fatalf("expected out-of-order sequence 8 within ack window to be accepted")
	}
	if _, accepted = server.acceptClientRequest(addr, 9, 0, 0); !accepted {
		t.Fatalf("expected out-of-order sequence 9 within ack window to be accepted")
	}

	ack, ackBits := server.clientAckState(client)
	if ack != 10 {
		t.Fatalf("expected ack 10, got %d", ack)
	}
	if ackBits&0b11 != 0b11 {
		t.Fatalf("expected ack bits for sequences 9 and 8, got %064b", ackBits)
	}

	if _, accepted = server.acceptClientRequest(addr, 9, 0, 0); accepted {
		t.Fatalf("expected duplicate sequence 9 to be dropped")
	}
}

func TestUDPClientStoresPeerAckState(t *testing.T) {
	server := NewUDPServer(NewRuntime(Config{}))
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:45679")
	if err != nil {
		t.Fatalf("resolve addr: %v", err)
	}

	client, accepted := server.acceptClientRequest(addr, 1, 77, 0b101)
	if !accepted {
		t.Fatalf("expected sequence to be accepted")
	}
	if client.peerAck != 77 || client.peerAckBits != 0b101 {
		t.Fatalf("unexpected peer ack state: ack=%d bits=%b", client.peerAck, client.peerAckBits)
	}
}
