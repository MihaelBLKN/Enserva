# Quick Start

This page gets the included UDP host running and shows the smallest useful interaction path.

## 1. Start the Server

```bash
go run .
```

Expected startup logs include the UDP bind address and configured tick/snapshot rates:

```text
Enserva UDP API running on :9000
Tick rate: 128/s, snapshots: 20/s
```

!!! tip
Use `go run . -udpPort 9100` if another process is already using port `9000`.

## 2. Authenticate a UDP Client

When the sample objects are enabled, Enserva registers a `PlayerAuthenticator`. Send an authentication datagram:

```json
{
  "type": "auth",
  "seq": 1,
  "data": {}
}
```

The sample authenticator creates a player object and returns the assigned ID:

```json
{
  "type": "auth",
  "seq": 1,
  "ok": true,
  "clientId": "player-1",
  "authenticatedId": "player-1"
}
```

## 3. Send Player Input

After authentication, send requests to the created player object:

```json
{
  "seq": 2,
  "objectType": "player",
  "objectId": "player-1",
  "action": "input",
  "data": {
    "x": 1,
    "y": 0
  }
}
```

The player clamps each velocity axis to `[-1, 1]` and moves on each runtime tick according to its configured speed.

## 4. Receive Snapshots

The UDP server periodically broadcasts snapshots to active clients:

```json
{
  "type": "snapshot",
  "clientId": "player-1",
  "tick": 128,
  "lastSeq": 2,
  "objects": {
    "player": {
      "player-1": {
        "id": "player-1",
        "x": 180,
        "y": 0,
        "velocityX": 1,
        "velocityY": 0,
        "speed": 180
      }
    }
  }
}
```

!!! note
If an authentication object is registered, unauthenticated UDP clients do not receive snapshots and regular object requests are rejected.

## Minimal Server Code

```go
package main

import (
	"log"

	netobjects "Enserva/netObjects"
	"Enserva/network"
)

func main() {
	config := network.DefaultConfig()
	config.UDPAddress = ":9000"

	server := network.NewServer(config)
	if err := netobjects.Register(server); err != nil {
		log.Fatal(err)
	}

	log.Fatal(server.ListenAndServe())
}
```
