# Enserva

Enserva is an early Go networking runtime for authoritative multiplayer servers and server-side simulations. It provides a small object model, a tick loop, UDP request routing, authentication hooks, and snapshot broadcasting while leaving game rules fully in your code. The goal is to support competitive games and other game-server experiments.

The core idea is simple: your server owns the world. You define objects such as players, buildings, projectiles, doors, or match state, register them with the runtime, and decide how they respond to ticks, requests, authentication, and snapshots. Clients can send requests to existing objects, but they cannot create authoritative state by inventing object IDs.

Enserva's primary client protocol is the binary wire protocol: buffer-backed wire packets documented in the [Wire Protocol](https://mihaelblkn.github.io/Enserva/api/wire-protocol/) guide. Legacy JSON datagrams are still supported for compatibility, debugging, and simple tools, but new clients should send wire packets and register typed game messages for hot-path gameplay.

## Features

- Authoritative server-side object registry.
- Configurable tick rate and snapshot rate.
- Optional object hooks for initialization, per-tick updates, once-per-second updates, client requests, authentication, and snapshot visibility.
- UDP transport with binary wire packets, request sequencing, client tracking, authentication gating, and legacy JSON compatibility.
- Server-side object factories for controlled object creation.
- Snapshot broadcasting grouped by object type and object ID.
- Per-client interest management for filtering snapshot objects with a spatial hash.
- Scene management for rooms, maps, shards, scene switches, and scene-filtered snapshots.
- Optional browser debug UI for runtime state, config, UDP clients, features, and object snapshots.
- Example `Player`, `Building`, and authentication objects.

## Project Status

Enserva is still a rough sketch of a game server framework. The API is useful for experimentation and iteration, but it should not be treated as stable yet.

## Basic Usage

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

Run the included example host:

```bash
go run .
```

Useful flags include `-udpPort`, `-tickRate`, `-snapshotRate`, `-clientTimeout`, `-exampleObjects`, `-debug`, and `-debugAddr`.

Launch the debug interface:

```bash
go run . -debug
```

Then open `http://localhost:9100`.

## Documentation

The full docs live in `documentation/docs` and can be built with MkDocs Material. Start with:

- [Quick Start](https://mihaelblkn.github.io/Enserva/quick-start/)
- [Configuration](https://mihaelblkn.github.io/Enserva/configuration/)
- [Architecture](https://mihaelblkn.github.io/Enserva/architecture/)
- [Features](https://mihaelblkn.github.io/Enserva/features/interest-management/)
- [Scenes](https://mihaelblkn.github.io/Enserva/guides/scenes/)
- [Wire Protocol](https://mihaelblkn.github.io/Enserva/api/wire-protocol/)
- [API Reference](https://mihaelblkn.github.io/Enserva/developer-api/)

## License

Enserva is licensed under the MIT license.
