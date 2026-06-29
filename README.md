# Enserva

Enserva is a small Go networking API for tick-based multiplayer/server simulations.

The server core does not know about your world rules. Server code defines network objects, registers them, and creates them. Client requests can only target objects that already exist.

# NOTICE

This is still very rough of a sketch, there's still a long way to go on this game server.

## Core Idea

Create a normal Go package in your app, for example `netObjects`, and define objects like `player.go`, `building.go`, `projectile.go`, or anything else your game needs. Objects are authoritative server state; a client cannot invent a new `objectId` and make the server spawn it.

Each object can implement:

- `ObjectType() string`
- `ObjectID() string`
- `Snapshot() any`
- `OnTick(network.TickContext)`
- `OnFullTick(network.TickContext)`
- `OnRequest(network.RequestContext) error`
- `OnAuthenticationAttempt(network.AuthenticationContext) (string, error)`
- `SnapshotVisible() bool`

`OnTick` runs every simulation tick.
`OnFullTick` runs once per completed second of ticks. With the default `128` tick rate, it runs after every `128` ticks.
`OnRequest` runs when a UDP request targets that object.
`OnAuthenticationAttempt` is optional and can be bound to exactly one object with `RegisterAuthenticationObject`.
`SnapshotVisible` is optional and can hide server-only objects such as authentication handlers.

## Example Object Registration

```go
server := network.NewServer(network.Config{
	UDPAddress:   ":9000",
	TickRate:     128,
	SnapshotRate: 20,
})

authenticator := netobjects.NewPlayerAuthenticator("default")
if err := server.RegisterAuthenticationObject(authenticator); err != nil {
	log.Fatal(err)
}

if err := server.RegisterFactory("player", network.ObjectFactoryFunc(netobjects.PlayerFactory)); err != nil {
	log.Fatal(err)
}
if err := server.RegisterFactory("building", network.ObjectFactoryFunc(netobjects.BuildingFactory)); err != nil {
	log.Fatal(err)
}
if _, err := server.CreateObject("building", "building-1"); err != nil {
	log.Fatal(err)
}

log.Fatal(server.ListenAndServe())
```

The included `netObjects` package is only an example of how you can define your objects. Factories are server-side helpers only. Registering a factory does not allow clients to create missing objects.

## Player Authentication

Authentication is handled by a normal developer-owned network object. That object implements `OnAuthenticationAttempt`, validates credentials on the server, creates any server-owned objects it wants, and returns the ID that should identify the UDP connection.

Only one object can be bound as the authentication handler:

```go
type PlayerAuthenticator struct {
	ID           string
	NextPlayerID uint64
}

func (auth *PlayerAuthenticator) ObjectType() string {
	return "player-auth"
}

func (auth *PlayerAuthenticator) ObjectID() string {
	return auth.ID
}

func (auth *PlayerAuthenticator) Snapshot() any {
	return nil
}

func (auth *PlayerAuthenticator) SnapshotVisible() bool {
	return false
}

func (auth *PlayerAuthenticator) OnAuthenticationAttempt(ctx network.AuthenticationContext) (string, error) {
	var payload struct {
		Token string `json:"token"`
	}
	if err := ctx.Decode(&payload); err != nil {
		return "", err
	}

	accountID, err := authenticateToken(payload.Token)
	if err != nil {
		return "", err
	}

	playerID := "player-" + accountID
	player := NewPlayer(playerID)
	player.AssignClient(playerID)

	if err := ctx.Runtime.RegisterObject(player); err != nil {
		return "", err
	}

	return playerID, nil
}
```

Clients authenticate with a transport-level auth message:

```json
{
  "type": "auth",
  "seq": 1,
  "data": {
    "token": "client-token"
  }
}
```

On success, UDP clients receive a direct response:

```json
{
  "type": "auth",
  "seq": 1,
  "ok": true,
  "clientId": "player-42",
  "authenticatedId": "player-42"
}
```

After that, the UDP connection's `ClientID` is `player-42`, and regular object requests can use that identity for authorization. If an authentication object is registered, unauthenticated UDP clients do not receive snapshots and regular requests are rejected until authentication succeeds.

## Request Format

Send each request as a JSON UDP datagram:

```json
{
  "seq": 1,
  "objectType": "player",
  "objectId": "player-1",
  "action": "move",
  "data": {
    "x": 1,
    "y": 0
  }
}
```

Requests only route to existing objects. If the object does not exist, the server returns an error response and does not call a factory. To create an object from a registered factory, call `server.CreateObject(objectType, objectID)` from server code.

## Snapshot Format

Clients receive snapshots as JSON UDP datagrams:

```json
{
  "type": "snapshot",
  "clientId": "player-1",
  "tick": 128,
  "lastSeq": 1,
  "objects": {
    "player": {
      "player-1": {
        "id": "player-1",
        "x": 180,
        "y": 0
      }
    }
  }
}
```

## Run The Example Host

```bash
go run .
```

Useful flags:

- `-udpPort`
- `-tickRate`
- `-snapshotRate`
- `-clientTimeout`
- `-exampleObjects`

Enserva starts only the UDP listener. Clients must connect to the configured UDP port directly.
