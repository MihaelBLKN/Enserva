# Enserva

Enserva is a small Go networking API for tick-based multiplayer and server simulations. It provides an authoritative runtime, a UDP transport, and a lightweight object model that lets server code decide which objects exist and how clients may interact with them.

!!! warning "Project status"
Enserva is still an early project. Treat the documented APIs as a guide to the present release, not a long-term compatibility promise.

## Features

- Authoritative server-side object registry.
- Tick loop with per-tick and full-second hooks.
- UDP request routing to existing objects.
- Snapshot broadcasting with per-object visibility controls.
- Optional object-based authentication flow.
- Server-side object factories for controlled creation.
- Example `player`, `building`, and authenticator objects.

## Quick Installation

```bash
git clone https://github.com/MihaelBLKN/Enserva.git
cd Enserva
go test ./...
go run .
```

The example host starts a UDP server on port `9000` by default.

## Quick Example

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

## Documentation Map

| Section                           | Purpose                                                           |
| --------------------------------- | ----------------------------------------------------------------- |
| [Installation](installation.md)   | Toolchain, clone, build, test, and local docs setup.              |
| [Quick Start](quick-start.md)     | Start the included host and send a first UDP message.             |
| [Configuration](configuration.md) | Runtime config, CLI flags, UDP messages, and supported options.   |
| [Architecture](architecture.md)   | Runtime layout, request flow, snapshots, and concurrency model.   |
| [Developer API](developer-api.md) | Public package overview and links to detailed package references. |
| [Examples](examples.md)           | Tutorials built from the README, tests, and sample objects.       |
| [FAQ](faq.md)                     | Common usage and design questions.                                |
| [Changelog](changelog.md)         | Release notes and project history.                                |
