package tests

import (
	"Enserva/network"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type authenticationTestObject struct {
	id       string
	returnID string
	calls    int
}

// ObjectType returns the authentication object type used by tests.
func (object *authenticationTestObject) ObjectType() string {
	return "auth"
}

// ObjectID returns the configured test object id.
func (object *authenticationTestObject) ObjectID() string {
	return object.id
}

// Snapshot returns no state for the authentication test object.
func (object *authenticationTestObject) Snapshot() any {
	return nil
}

// OnAuthenticationAttempt accepts the test token and returns the configured id.
func (object *authenticationTestObject) OnAuthenticationAttempt(ctx network.AuthenticationContext) (string, error) {
	object.calls++

	var payload struct {
		Token string `json:"token"`
	}
	if err := ctx.Decode(&payload); err != nil {
		return "", err
	}
	if payload.Token != "valid-token" {
		return "", errors.New("invalid token")
	}

	return object.returnID, nil
}

type routeClientTestObject struct {
	id       string
	clientID string
}

// ObjectType returns the request route object type used by tests.
func (object *routeClientTestObject) ObjectType() string {
	return "thing"
}

// ObjectID returns the configured route target id.
func (object *routeClientTestObject) ObjectID() string {
	return object.id
}

// Snapshot returns no state for the route target test object.
func (object *routeClientTestObject) Snapshot() any {
	return nil
}

// OnRequest records the client id routed by the runtime.
func (object *routeClientTestObject) OnRequest(ctx network.RequestContext) error {
	object.clientID = ctx.ClientID
	return nil
}

// TestRuntimeBindsOneAuthenticationObject verifies that a runtime owns one authentication handler.
func TestRuntimeBindsOneAuthenticationObject(t *testing.T) {
	runtime := network.NewRuntime(network.Config{})
	authenticator := &authenticationTestObject{id: "primary", returnID: "player-a"}
	if err := runtime.RegisterAuthenticationObject(authenticator); err != nil {
		t.Fatalf("register authentication object: %v", err)
	}

	err := runtime.RegisterAuthenticationObject(&authenticationTestObject{id: "secondary", returnID: "player-b"})
	if !errors.Is(err, network.ErrAuthenticationHandlerExists) {
		t.Fatalf("expected one authentication handler error, got %v", err)
	}

	authenticatedID, err := runtime.HandleAuthenticationAttempt(network.AuthenticationContext{
		Transport:  "test",
		ClientID:   "connection-a",
		ReceivedAt: time.Now(),
		Request: network.RequestMessage{
			Type: "auth",
			Data: json.RawMessage(`{"token":"valid-token"}`),
		},
	})
	if err != nil {
		t.Fatalf("authentication attempt: %v", err)
	}
	if authenticatedID != "player-a" {
		t.Fatalf("expected authenticated id player-a, got %q", authenticatedID)
	}
	if authenticator.calls != 1 {
		t.Fatalf("expected one authentication call, got %d", authenticator.calls)
	}

	runtime.RemoveObject("auth", "primary")
	if runtime.AuthenticationRequired() {
		t.Fatalf("expected authentication to unbind when auth object is removed")
	}
	if err := runtime.RegisterAuthenticationObject(&authenticationTestObject{id: "secondary", returnID: "player-b"}); err != nil {
		t.Fatalf("expected new authentication object after removal: %v", err)
	}
}

// TestAuthenticationReturnedIDAuthorizesLaterRequests verifies that authenticated ids are used for routing.
func TestAuthenticationReturnedIDAuthorizesLaterRequests(t *testing.T) {
	runtime := network.NewRuntime(network.Config{})
	if err := runtime.RegisterAuthenticationObject(&authenticationTestObject{id: "primary", returnID: "player-a"}); err != nil {
		t.Fatalf("register authentication object: %v", err)
	}

	routeTarget := &routeClientTestObject{id: "alpha"}
	if err := runtime.RegisterObject(routeTarget); err != nil {
		t.Fatalf("register route target: %v", err)
	}

	authenticatedID, err := runtime.HandleAuthenticationAttempt(network.AuthenticationContext{
		Transport: "test",
		ClientID:  "connection-a",
		Request: network.RequestMessage{
			Type: "auth",
			Data: json.RawMessage(`{"token":"valid-token"}`),
		},
	})
	if err != nil {
		t.Fatalf("handle auth request: %v", err)
	}
	if authenticatedID != "player-a" {
		t.Fatalf("expected authenticated id player-a, got %q", authenticatedID)
	}

	err = runtime.HandleRequest(network.RequestContext{
		ClientID: authenticatedID,
		Request: network.RequestMessage{
			ObjectType: "thing",
			ObjectID:   "alpha",
		},
	})
	if err != nil {
		t.Fatalf("handle authenticated request: %v", err)
	}
	if routeTarget.clientID != "player-a" {
		t.Fatalf("expected routed request client id player-a, got %q", routeTarget.clientID)
	}
}
