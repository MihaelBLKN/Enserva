# Enserva

Enserva is a small Go networking API for tick-based multiplayer/server simulations.

The server core does not know about players, buildings, anti-cheat, movement, collisions, or world rules. External stuff defines their own network objects and register them with the server.

# NOTICE

This is still very rough of a sketch, there's still a long way to go on this game server.

## Core Idea

Create a normal Go package in your app, for example `netObjects`, and define objects like `player.go`, `building.go`, `projectile.go`, or anything else your game needs.

Each object can implement:

- `ObjectType() string`
- `ObjectID() string`
- `Snapshot() any`
- `OnTick(network.TickContext)`
- `OnFullTick(network.TickContext)`
- `OnRequest(network.RequestContext) error`

`OnTick` runs every simulation tick.
`OnFullTick` runs once per completed second of ticks. With the default `128` tick rate, it runs after every `128` ticks.
`OnRequest` runs when a UDP request targets that object.

## Example Object Registration

```go
server := network.NewServer(network.Config{
	UDPAddress:   ":9000",
	TickRate:     128,
	SnapshotRate: 20,
})

server.RegisterFactory("player", network.ObjectFactoryFunc(netobjects.PlayerFactory))
server.RegisterFactory("building", network.ObjectFactoryFunc(netobjects.BuildingFactory))

log.Fatal(server.ListenAndServe())
```

The included `netObjects` package is only an example of how you can define your objects.

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

If a factory is registered for `objectType`, the server can create missing object IDs automatically before calling `OnRequest`.

## Snapshot Format

Clients receive snapshots as JSON UDP datagrams:

```json
{
  "type": "snapshot",
  "clientId": "udp-127.0.0.1:50000",
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
