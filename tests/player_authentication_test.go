package tests

import (
	netobjects "Enserva/netObjects"
	"Enserva/network"
	"encoding/json"
	"errors"
	"testing"
)

func TestPlayerAuthenticatorCreatesOwnedPlayer(t *testing.T) {
	server := network.NewServer(network.Config{})
	authenticator := netobjects.NewPlayerAuthenticator("default")
	if err := server.RegisterAuthenticationObject(authenticator); err != nil {
		t.Fatalf("register authentication object: %v", err)
	}

	playerID, err := server.Runtime().HandleAuthenticationAttempt(network.AuthenticationContext{
		Transport: "test",
		ClientID:  "pending-client",
		Request: network.RequestMessage{
			Type: "auth",
		},
	})
	if err != nil {
		t.Fatalf("authentication attempt: %v", err)
	}
	if playerID != "player-1" {
		t.Fatalf("expected player-1, got %q", playerID)
	}

	object, ok := server.Runtime().GetObject("player", playerID)
	if !ok {
		t.Fatalf("expected authenticated player object")
	}

	player := object.(*netobjects.Player)
	if player.OwnerClientID != playerID {
		t.Fatalf("expected player owner %q, got %q", playerID, player.OwnerClientID)
	}
	initialSpeed := player.Speed

	snapshot := server.Runtime().Snapshot()
	if _, ok := snapshot["player-auth"]; ok {
		t.Fatalf("authenticator leaked into snapshot: %#v", snapshot)
	}

	err = server.Runtime().HandleRequest(network.RequestContext{
		ClientID: playerID,
		Request: network.RequestMessage{
			ObjectType: "player",
			ObjectID:   playerID,
			Action:     "input",
			Data:       json.RawMessage(`{"x":1,"speed":999}`),
		},
	})
	if err != nil {
		t.Fatalf("expected owner input to succeed: %v", err)
	}
	if player.VelocityX != 1 {
		t.Fatalf("expected input velocity x=1, got %v", player.VelocityX)
	}
	if player.Speed != initialSpeed {
		t.Fatalf("expected client input not to change speed, got %v", player.Speed)
	}

	err = server.Runtime().HandleRequest(network.RequestContext{
		ClientID: "different-client",
		Request: network.RequestMessage{
			ObjectType: "player",
			ObjectID:   playerID,
			Action:     "input",
			Data:       json.RawMessage(`{"x":1}`),
		},
	})
	if !errors.Is(err, netobjects.ErrUnauthorizedPlayerClient) {
		t.Fatalf("expected unauthorized player request, got %v", err)
	}
}
