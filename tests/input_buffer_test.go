package tests

import (
	"Enserva/network"
	"errors"
	"testing"
)

func TestInputForCurrentTickIsConsumable(t *testing.T) {
	runtime := network.NewRuntime(network.Config{})

	if err := runtime.BufferClientInput(network.ClientInput{
		ClientID:   "client-a",
		Sequence:   7,
		ObjectType: "player",
		ObjectID:   "player-1",
		Payload:    "move",
	}); err != nil {
		t.Fatalf("buffer input: %v", err)
	}

	inputs := runtime.ConsumeClientInputs("client-a")
	if len(inputs) != 1 {
		t.Fatalf("expected one input, got %d", len(inputs))
	}
	if inputs[0].Tick != runtime.Tick() || inputs[0].Sequence != 7 {
		t.Fatalf("unexpected input: %#v", inputs[0])
	}
	if runtime.InputBufferMetrics().Consumed != 1 {
		t.Fatalf("expected consumed counter to increment")
	}
}

func TestFutureInputIsBufferedUntilTickArrives(t *testing.T) {
	runtime := network.NewRuntime(network.Config{MaxInputFutureTicks: 4})

	if err := runtime.BufferClientInput(network.ClientInput{
		ClientID: "client-a",
		Sequence: 1,
		Tick:     2,
		ObjectID: "player-1",
	}); err != nil {
		t.Fatalf("buffer future input: %v", err)
	}
	if got := runtime.ConsumeClientInputs("client-a"); len(got) != 0 {
		t.Fatalf("expected no current inputs, got %#v", got)
	}

	runtime.Advance()
	runtime.Advance()

	inputs := runtime.ConsumeClientInputs("client-a")
	if len(inputs) != 1 || inputs[0].Tick != 2 {
		t.Fatalf("expected future input at tick 2, got %#v", inputs)
	}
}

func TestStaleInputIsRejected(t *testing.T) {
	runtime := network.NewRuntime(network.Config{MaxInputPastTicks: 2})
	for index := 0; index < 5; index++ {
		runtime.Advance()
	}

	err := runtime.BufferClientInput(network.ClientInput{
		ClientID: "client-a",
		Sequence: 1,
		Tick:     2,
		ObjectID: "player-1",
	})
	if !errors.Is(err, network.ErrStaleClientInput) {
		t.Fatalf("expected stale input error, got %v", err)
	}
	if runtime.InputBufferMetrics().StaleRejected != 1 {
		t.Fatalf("expected stale rejected counter")
	}
}

func TestTooFarFutureInputIsRejected(t *testing.T) {
	runtime := network.NewRuntime(network.Config{MaxInputFutureTicks: 2})

	err := runtime.BufferClientInput(network.ClientInput{
		ClientID: "client-a",
		Sequence: 1,
		Tick:     3,
		ObjectID: "player-1",
	})
	if !errors.Is(err, network.ErrFutureClientInput) {
		t.Fatalf("expected future input error, got %v", err)
	}
	if runtime.InputBufferMetrics().FutureRejected != 1 {
		t.Fatalf("expected future rejected counter")
	}
}

func TestInputsAreReturnedInDeterministicSequenceOrder(t *testing.T) {
	runtime := network.NewRuntime(network.Config{})

	for _, sequence := range []uint64{3, 1, 2} {
		if err := runtime.BufferClientInput(network.ClientInput{
			ClientID: "client-a",
			Sequence: sequence,
			ObjectID: "player-1",
		}); err != nil {
			t.Fatalf("buffer input %d: %v", sequence, err)
		}
	}

	inputs := runtime.ConsumeClientInputs("client-a")
	if len(inputs) != 3 {
		t.Fatalf("expected three inputs, got %d", len(inputs))
	}
	for index, want := range []uint64{1, 2, 3} {
		if inputs[index].Sequence != want {
			t.Fatalf("input %d: expected sequence %d, got %d", index, want, inputs[index].Sequence)
		}
	}
}

func TestInputBufferLimitIsEnforced(t *testing.T) {
	runtime := network.NewRuntime(network.Config{InputBufferLimit: 2})

	for _, sequence := range []uint64{3, 1, 2} {
		if err := runtime.BufferClientInput(network.ClientInput{
			ClientID: "client-a",
			Sequence: sequence,
			ObjectID: "player-1",
		}); err != nil {
			t.Fatalf("buffer input %d: %v", sequence, err)
		}
	}

	inputs := runtime.ConsumeClientInputs("client-a")
	if len(inputs) != 2 {
		t.Fatalf("expected two retained inputs, got %#v", inputs)
	}
	if inputs[0].Sequence != 2 || inputs[1].Sequence != 3 {
		t.Fatalf("expected retained sequence order 2,3 got %#v", inputs)
	}
	if runtime.InputBufferMetrics().Dropped != 1 {
		t.Fatalf("expected one dropped input")
	}
}

func TestPlayerInputWireCompatibilityAcceptsLegacyPayload(t *testing.T) {
	legacyPayload := append([]byte{0, byte(len("player-1"))}, []byte("player-1")...)
	legacyPayload = append(legacyPayload,
		0x3f, 0x80, 0x00, 0x00,
		0xbf, 0x00, 0x00, 0x00,
		0x3e, 0x80, 0x00, 0x00,
	)

	input, err := network.DecodePlayerInput(legacyPayload)
	if err != nil {
		t.Fatalf("decode legacy player input: %v", err)
	}
	if input.Sequence != 0 || input.Tick != 0 || input.ObjectID != "player-1" || input.X != 1 || input.Y != -0.5 || input.Z != 0.25 {
		t.Fatalf("unexpected legacy input: %#v", input)
	}
}
