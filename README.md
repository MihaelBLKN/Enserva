# Enserva

Enserva is an early Go networking runtime for authoritative multiplayer servers and server-side simulations. It provides a small object model, a tick loop, UDP request routing, authentication hooks, and snapshot broadcasting while leaving game rules fully in your code. The goal is to support competitive games and other game-server experiments.

The core idea is simple: your server owns the world. You define objects such as players, buildings, projectiles, doors, or match state, register them with the runtime, and decide how they respond to ticks, requests, authentication, and snapshots. Clients can send requests to existing objects, but they cannot create authoritative state by inventing object IDs.

## Features

- Authoritative server-side object registry.
- Configurable tick rate and snapshot rate.
- Optional object hooks for initialization, per-tick updates, once-per-second updates, client requests, authentication, and snapshot visibility.
- UDP transport with request sequencing, client tracking, authentication gating, and JSON messages.
- Server-side object factories for controlled object creation.
- Snapshot broadcasting grouped by object type and object ID.
- Per-client interest management for filtering snapshot objects by distance.
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

Useful flags include `-udpPort`, `-tickRate`, `-snapshotRate`, `-clientTimeout`, and `-exampleObjects`.

## Documentation

The full docs live in `documentation/docs` and can be built with MkDocs Material. Start with:

- [Quick Start](https://mihaelblkn.github.io/Enserva/quick-start/)
- [Configuration](https://mihaelblkn.github.io/Enserva/configuration/)
- [Architecture](https://mihaelblkn.github.io/Enserva/architecture/)
- [Features](https://mihaelblkn.github.io/Enserva/features/interest-management/)
- [API Reference](https://mihaelblkn.github.io/Enserva/developer-api/)

## License

Enserva is licensed under the MIT license.
