# Examples

These examples are derived from the README, sample objects, and tests in the repository.

## Register the Sample Objects

```go
package main

import (
	"log"

	netobjects "Enserva/netObjects"
	"Enserva/network"
)

func main() {
	server := network.NewServer(network.DefaultConfig())

	if err := netobjects.Register(server); err != nil {
		log.Fatal(err)
	}

	log.Fatal(server.ListenAndServe())
}
```

`netobjects.Register` binds the sample authenticator and factories for `player` and `building`.

## Create Server-Owned Objects

Factories are called only by server code:

```go
server := network.NewServer(network.DefaultConfig())

if err := server.RegisterFactory("building", network.ObjectFactoryFunc(netobjects.BuildingFactory)); err != nil {
	return err
}

building, err := server.CreateObject("building", "building-1")
if err != nil {
	return err
}

_ = building
```

!!! warning
A client request to a missing `building-1` returns `ErrObjectNotFound`; it does not call `BuildingFactory`.

## Write a Custom Object

```go
type Door struct {
	ID     string `json:"id"`
	Open   bool   `json:"open"`
	Uses   uint64 `json:"uses"`
	Client string `json:"client,omitempty"`
}

func (door *Door) ObjectType() string { return "door" }
func (door *Door) ObjectID() string   { return door.ID }
func (door *Door) Snapshot() any      { return *door }

func (door *Door) OnRequest(ctx network.RequestContext) error {
	switch ctx.Request.Action {
	case "open":
		door.Open = true
	case "close":
		door.Open = false
	default:
		return nil
	}

	door.Uses++
	door.Client = ctx.ClientID
	return nil
}
```

Register it:

```go
door := &Door{ID: "door-1"}
if err := server.RegisterObject(door); err != nil {
	return err
}
```

Send a request:

```json
{
  "seq": 10,
  "objectType": "door",
  "objectId": "door-1",
  "action": "open"
}
```

## Authenticate with a Domain Object

The tests show the intended authentication shape: an object decodes credentials, validates them, and returns the ID that should represent the client.

```go
type Authenticator struct {
	ID string
}

func (auth *Authenticator) ObjectType() string { return "auth" }
func (auth *Authenticator) ObjectID() string   { return auth.ID }
func (auth *Authenticator) Snapshot() any      { return nil }
func (auth *Authenticator) SnapshotVisible() bool {
	return false
}

func (auth *Authenticator) OnAuthenticationAttempt(ctx network.AuthenticationContext) (string, error) {
	var payload struct {
		Token string `json:"token"`
	}
	if err := ctx.Decode(&payload); err != nil {
		return "", err
	}
	if payload.Token != "valid-token" {
		return "", errors.New("invalid token")
	}

	return "player-a", nil
}
```

Register it:

```go
if err := server.RegisterAuthenticationObject(&Authenticator{ID: "primary"}); err != nil {
	return err
}
```

## Test Request Routing Without UDP

You can test object behavior directly through `Runtime.HandleRequest`:

```go
runtime := network.NewRuntime(network.Config{})
player := netobjects.NewPlayer("player-1")
player.AssignClient("player-1")

if err := runtime.RegisterObject(player); err != nil {
	t.Fatal(err)
}

err := runtime.HandleRequest(network.RequestContext{
	ClientID: "player-1",
	Request: network.RequestMessage{
		ObjectType: "player",
		ObjectID:   "player-1",
		Action:     "input",
		Data:       json.RawMessage(`{"x":1,"y":0}`),
	},
})
if err != nil {
	t.Fatal(err)
}
```

This pattern avoids UDP timing and lets tests assert object state directly.

## Use Direct Responses

Objects can send an immediate response when the transport supplies a `ResponseWriter`:

```go
func (door *Door) OnRequest(ctx network.RequestContext) error {
	door.Open = true

	return ctx.Respond(network.ResponseMessage{
		Type:     "door",
		Sequence: ctx.Request.Sequence,
		OK:       true,
		Data: map[string]any{
			"open": door.Open,
		},
	})
}
```

!!! note
Direct responses are optional. `ctx.Respond` returns `ErrResponsesUnsupported` if no response writer is available.
