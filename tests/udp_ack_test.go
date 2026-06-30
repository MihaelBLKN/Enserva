package tests

import (
	"Enserva/network"
	"testing"
)

func TestWirePacketAckStateRoundTrip(t *testing.T) {
	message, err := network.EncodeClientMessage(network.Ping{Nonce: 42})
	if err != nil {
		t.Fatalf("encode ping: %v", err)
	}

	packet, err := network.EncodePacketWithAcks(10, 77, 0b101, []network.WireMessage{message})
	if err != nil {
		t.Fatalf("encode packet: %v", err)
	}

	decoded, err := network.DecodePacket(packet)
	if err != nil {
		t.Fatalf("decode packet: %v", err)
	}
	if decoded.Sequence != 10 {
		t.Fatalf("expected sequence 10, got %d", decoded.Sequence)
	}
	if decoded.Ack != 77 || decoded.AckBits != 0b101 {
		t.Fatalf("unexpected ack state: ack=%d bits=%b", decoded.Ack, decoded.AckBits)
	}
}
