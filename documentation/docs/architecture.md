# Architecture

Enserva is organized around a small authoritative runtime. The runtime owns object identity, object lifecycle, request routing, ticks, and snapshots. The UDP server is a transport adapter around that runtime.

## Package Layout

```text
Enserva/
├── main.go                 # Example UDP host and CLI flags
├── network/
│   ├── protocol.go         # Config, interfaces, message/context types
│   ├── runtime.go          # Runtime registry, hooks, auth, snapshots
│   ├── server.go           # Server facade
│   └── udp.go              # UDP transport
├── netObjects/
│   ├── player.go           # Sample player and authenticator
│   ├── building.go         # Sample building object
│   └── register.go         # Sample package registration helper
└── tests/                  # Runtime/authentication behavior tests
```

## Component Model

```mermaid
flowchart TD
    Main["main.go"] --> Server["network.Server"]
    Server --> Runtime["network.Runtime"]
    Server --> UDP["network.UDPServer"]
    UDP --> Runtime
    Runtime --> Registry["Object registry"]
    Runtime --> Factories["Factory registry"]
    Runtime --> Auth["Authentication handler"]
    Runtime --> Hooks["Tick/request hooks"]
    NetObjects["netObjects examples"] --> Registry
    NetObjects --> Factories
    NetObjects --> Auth
```

## Execution Flow

1. Application code creates a `network.Server`.
2. Application code registers objects and factories.
3. Optional authentication object is registered.
4. `ListenAndServe` starts the UDP listener.
5. A goroutine advances runtime ticks at `Config.TickInterval()`.
6. UDP datagrams are decoded into `RequestMessage`.
7. Authentication messages go to the authentication object.
8. Regular messages route to existing objects by `objectType` and `objectId`.
9. Snapshots are broadcast every `Config.SnapshotEvery()` ticks.

## Request Routing

```mermaid
sequenceDiagram
    participant Client
    participant UDP as UDPServer
    participant Runtime
    participant Object

    Client->>UDP: JSON UDP datagram
    UDP->>UDP: accept client and sequence
    alt authentication message
        UDP->>Runtime: HandleAuthenticationAttempt
        Runtime->>Object: OnAuthenticationAttempt
        Object-->>Runtime: authenticated ID
        Runtime-->>UDP: authenticated ID
        UDP-->>Client: AuthenticationResponse
    else object request
        UDP->>Runtime: HandleRequest
        Runtime->>Runtime: lookup existing object
        Runtime->>Object: OnRequest
        Object-->>Runtime: error or nil
        Runtime-->>UDP: error or nil
    end
```

!!! important
Client requests never call factories. A request must target an object that already exists in the runtime registry.

## Tick and Snapshot Flow

```mermaid
flowchart LR
    Tick["Ticker fires"] --> Advance["Runtime.Advance"]
    Advance --> OnTick["OnTick for tick handlers"]
    Advance --> Full{"tick % TickRate == 0?"}
    Full -->|yes| OnFullTick["OnFullTick for full-tick handlers"]
    Full -->|no| SnapshotCheck
    OnFullTick --> SnapshotCheck{"tick % SnapshotEvery == 0?"}
    SnapshotCheck -->|yes| Snapshot["Runtime.Snapshot"]
    Snapshot --> Broadcast["UDP snapshot broadcast"]
    SnapshotCheck -->|no| Done["Wait for next tick"]
```

## Object Identity

Objects are addressed by `(ObjectType, ObjectID)`. The runtime trims whitespace and rejects empty values. Object replacement is allowed through `RegisterObject`; controlled creation through `CreateObject` rejects duplicates.

## Authentication Model

Authentication is implemented by a normal object that also implements `AuthenticationHandler`. The runtime allows exactly one authentication object at a time.

When authentication is required:

- UDP auth messages are routed to the authentication object.
- The handler returns the authenticated client ID.
- The UDP client is marked authenticated under that ID.
- Regular requests from unauthenticated clients are rejected.
- Snapshot broadcasts skip unauthenticated clients.

## Concurrency Model

`Runtime` uses two locks:

| Lock      | Purpose                                                                                                  |
| --------- | -------------------------------------------------------------------------------------------------------- |
| `mu`      | Protects tick value, object registry, factory registry, and authentication handler fields.               |
| `hooksMu` | Serializes hook execution for `Advance`, `HandleRequest`, `HandleAuthenticationAttempt`, and `Snapshot`. |

The UDP server also has its own mutex for the client map.

!!! note
Hook serialization means object callbacks are not called concurrently by the runtime. Object code can still call back into runtime methods, but long-running hooks will delay ticks, requests, authentication, and snapshots.

## Extension Points

Use these extension points for application behavior:

| Extension point                 | Use it for                                                |
| ------------------------------- | --------------------------------------------------------- |
| `network.Object`                | Defining authoritative server state.                      |
| `network.RequestHandler`        | Handling client actions.                                  |
| `network.TickHandler`           | Movement, timers, physics steps, and per-tick simulation. |
| `network.FullTickHandler`       | Once-per-second counters and lower-frequency behavior.    |
| `network.AuthenticationHandler` | Mapping transport connections to application identities.  |
| `network.ObjectFactory`         | Server-controlled creation of objects by type and ID.     |
