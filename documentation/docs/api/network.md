# Package `network`

`Enserva/network` is the core package. It owns the runtime object registry, tick advancement, request routing, authentication hook, snapshot generation, and UDP server.

```go
import "Enserva/network"
```

## Configuration

### `Config`

```go
type Config struct {
	UDPAddress    string
	TickRate      int
	SnapshotRate  int
	ClientTimeout time.Duration
}
```

`Config` controls both the runtime and UDP server. Use `DefaultConfig()` when you want the repository defaults, then override fields:

```go
config := network.DefaultConfig()
config.UDPAddress = ":9100"
config.TickRate = 60
config.SnapshotRate = 10
```

Methods:

| Method            | Returns         | Notes                                                   |
| ----------------- | --------------- | ------------------------------------------------------- |
| `DefaultConfig()` | `Config`        | `:9000`, `128` ticks/s, `20` snapshots/s, `5s` timeout. |
| `Normalized()`    | `Config`        | Applies defaults and clamps snapshot rate to tick rate. |
| `TickInterval()`  | `time.Duration` | Duration between calls to `Runtime.Advance`.            |
| `SnapshotEvery()` | `uint64`        | Tick interval between UDP snapshot broadcasts.          |

## Core Object Interfaces

### `Object`

```go
type Object interface {
	ObjectType() string
	ObjectID() string
	Snapshot() any
}
```

Every registered object must provide a type, an ID, and a serializable snapshot. Object type and ID are trimmed before use.

### Optional Hooks

| Interface               | Method                                                           | Purpose                                                         |
| ----------------------- | ---------------------------------------------------------------- | --------------------------------------------------------------- |
| `InitHandler`           | `OnInit(InitContext)`                                            | Called immediately after an object is registered.               |
| `TickHandler`           | `OnTick(TickContext)`                                            | Called every tick after the runtime increments its tick number. |
| `FullTickHandler`       | `OnFullTick(TickContext)`                                        | Called when `tick % TickRate == 0`.                             |
| `RequestHandler`        | `OnRequest(RequestContext) error`                                | Called for requests targeting an existing object.               |
| `AuthenticationHandler` | `OnAuthenticationAttempt(AuthenticationContext) (string, error)` | Called for authentication messages.                             |
| `SnapshotVisibility`    | `SnapshotVisible() bool`                                         | Return `false` to exclude an object from snapshots.             |

## Factories

### `ObjectFactory`

```go
type ObjectFactory interface {
	CreateObject(RequestContext) (Object, error)
}
```

Factories are server-side helpers. Registering a factory does not allow a client to create an object by sending a request to a missing object.

### `ObjectFactoryFunc`

```go
type ObjectFactoryFunc func(RequestContext) (Object, error)
```

`ObjectFactoryFunc` adapts a function to `ObjectFactory`:

```go
server.RegisterFactory("player", network.ObjectFactoryFunc(func(ctx network.RequestContext) (network.Object, error) {
	return NewPlayer(ctx.Request.ObjectID), nil
}))
```

## Runtime

### `NewRuntime`

```go
runtime := network.NewRuntime(network.Config{})
```

Creates a runtime with normalized configuration and empty object/factory maps.

### Object Management

| Method                                                            | Purpose                                                             |
| ----------------------------------------------------------------- | ------------------------------------------------------------------- |
| `RegisterObject(object Object) error`                             | Adds or replaces an object at its `ObjectType()/ObjectID()` key.    |
| `RegisterAuthenticationObject(object Object) error`               | Registers an object and binds it as the single auth handler.        |
| `RemoveObject(objectType, objectID string)`                       | Removes an object. Removing the auth object unbinds authentication. |
| `GetObject(objectType, objectID string) (Object, bool)`           | Looks up a registered object.                                       |
| `RegisterFactory(objectType string, factory ObjectFactory) error` | Registers a factory for server-side object creation.                |
| `CreateObject(objectType, objectID string) (Object, error)`       | Creates and registers an object through a registered factory.       |

`CreateObject` validates that the factory returns an object with the requested type and ID.

```go
runtime := network.NewRuntime(network.Config{})
if err := runtime.RegisterFactory("building", network.ObjectFactoryFunc(BuildingFactory)); err != nil {
	return err
}

object, err := runtime.CreateObject("building", "building-1")
if err != nil {
	return err
}
_ = object
```

### Simulation and Requests

| Method                                                                   | Purpose                                                                                 |
| ------------------------------------------------------------------------ | --------------------------------------------------------------------------------------- |
| `Advance() uint64`                                                       | Increments the tick, calls `OnTick`, and calls `OnFullTick` once per configured second. |
| `HandleRequest(ctx RequestContext) error`                                | Routes a request to the existing target object.                                         |
| `HandleAuthenticationAttempt(ctx AuthenticationContext) (string, error)` | Invokes the registered authentication handler.                                          |
| `Snapshot() SnapshotData`                                                | Builds the nested snapshot map for visible objects.                                     |
| `SnapshotForClient(clientID string) SnapshotData`                        | Builds a client-specific snapshot when interest management is enabled.                  |
| `Tick() uint64`                                                          | Returns the current runtime tick.                                                       |
| `AuthenticationRequired() bool`                                          | Reports whether an auth handler is registered.                                          |
| `Config() Config`                                                        | Returns the normalized config.                                                          |
| `Features() *Features`                                                   | Returns the runtime feature registry.                                                   |

`HandleRequest` fills `ReceivedAt`, `Tick`, and `Runtime` on the context before invoking the object handler.

```go
err := runtime.HandleRequest(network.RequestContext{
	ClientID: "player-1",
	Request: network.RequestMessage{
		ObjectType: "player",
		ObjectID:   "player-1",
		Action:     "input",
		Data:       json.RawMessage(`{"x":1,"y":0}`),
	},
})
```

## Server

### `Server`

`Server` is a small facade over `Runtime` plus transport startup.

| Function or method                                               | Purpose                                                        |
| ---------------------------------------------------------------- | -------------------------------------------------------------- |
| `NewServer(config Config) *Server`                               | Creates a server with a new runtime.                           |
| `ListenAndServe(config Config) error`                            | Convenience function for `NewServer(config).ListenAndServe()`. |
| `Config() Config`                                                | Returns normalized configuration.                              |
| `Runtime() *Runtime`                                             | Exposes the underlying runtime.                                |
| `RegisterObject`, `RegisterAuthenticationObject`, `RemoveObject` | Delegate to the runtime.                                       |
| `RegisterFactory`, `CreateObject`                                | Delegate factory operations to the runtime.                    |
| `ListenAndServe() error`                                         | Starts the UDP listener.                                       |
| `ListenAndServeUDP() error`                                      | Starts the UDP listener explicitly.                            |

## UDP Server

### `NewUDPServer`

```go
udpServer := network.NewUDPServer(runtime)
err := udpServer.ListenAndServe()
```

`UDPServer` accepts JSON datagrams, tracks clients by UDP address, rejects duplicate or older non-zero sequence numbers, advances the runtime in a goroutine, and broadcasts snapshots at the configured rate.

!!! warning
`UDPClient` and `UDPServer` expose no public fields. Treat their internals as implementation details.

## Message Types

### `RequestMessage`

```go
type RequestMessage struct {
	Type       string          `json:"type,omitempty"`
	Sequence   uint64          `json:"seq,omitempty"`
	ObjectType string          `json:"objectType"`
	ObjectID   string          `json:"objectId"`
	Action     string          `json:"action,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
}
```

Used for both authentication and object requests.

### `SnapshotData` and `SnapshotMessage`

```go
type SnapshotData map[string]map[string]any
```

Snapshots are grouped by object type and object ID.

```go
type SnapshotMessage struct {
	Type         string       `json:"type"`
	ClientID     string       `json:"clientId,omitempty"`
	Tick         uint64       `json:"tick"`
	LastSequence uint64       `json:"lastSeq,omitempty"`
	Objects      SnapshotData `json:"objects"`
}
```

### `ResponseMessage`

```go
type ResponseMessage struct {
	Type     string `json:"type"`
	Sequence uint64 `json:"seq,omitempty"`
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	Data     any    `json:"data,omitempty"`
}
```

Used by the UDP transport for error responses and available to object handlers through `RequestContext.Respond`.

### `AuthenticationResponse`

```go
type AuthenticationResponse struct {
	Type            string `json:"type"`
	Sequence        uint64 `json:"seq,omitempty"`
	OK              bool   `json:"ok"`
	ClientID        string `json:"clientId"`
	AuthenticatedID string `json:"authenticatedId"`
}
```

Returned by the UDP server after successful authentication.

## Features

### Interest Management

Interest management is configured through `Runtime.Features()`:

```go
func (player *Player) OnInit(ctx network.InitContext) {
	ctx.Runtime().Features().EnableInterestManagement(
		network.PlayerInterest(player, "x", "y", "z", 750),
	)
}
```

Helper functions:

| Function                                          | Purpose                                      |
| ------------------------------------------------- | -------------------------------------------- |
| `PlayerInterest(object, x, y, z, radius)`         | Registers a 3D player/reference object.      |
| `PlayerInterest2D(object, x, y, radius)`          | Registers a 2D player/reference object.      |
| `GameObjectInterest(object, x, y, z)`             | Registers a 3D object that can be filtered.  |
| `GameObjectInterest2D(object, x, y)`              | Registers a 2D object that can be filtered.  |
| `EnableInterestManagement(InterestManagementConfig)` | Stores interest metadata for one object. |

See [Interest Management](../features/interest-management.md) for a full guide.

## Context Types

### `InitContext`

| Method         | Description                                      |
| -------------- | ------------------------------------------------ |
| `Object()`     | Object being initialized.                        |
| `ObjectType()` | Normalized object type used for registration.    |
| `ObjectID()`   | Normalized object ID used for registration.      |
| `Runtime()`    | Runtime that just registered the object.         |

### `TickContext`

| Field          | Description               |
| -------------- | ------------------------- |
| `Tick`         | Current tick number.      |
| `Delta`        | Tick duration.            |
| `DeltaSeconds` | Tick duration as seconds. |
| `Runtime`      | Runtime calling the hook. |
| `Features`     | Runtime feature registry. |

### `RequestContext`

| Field        | Description                                                    |
| ------------ | -------------------------------------------------------------- |
| `Transport`  | Transport name, such as `"udp"` in the built-in UDP server.    |
| `ClientID`   | Client identity assigned by the transport/authentication flow. |
| `Tick`       | Runtime tick when the request is routed.                       |
| `ReceivedAt` | Request timestamp.                                             |
| `Request`    | Parsed request message.                                        |
| `Runtime`    | Runtime routing the request.                                   |
| `Features`   | Runtime feature registry.                                      |
| `Response`   | Optional response writer.                                      |

Methods:

| Method                       | Purpose                                                              |
| ---------------------------- | -------------------------------------------------------------------- |
| `Decode(target any) error`   | Decodes `Request.Data` JSON into `target`; no-op when data is empty. |
| `Respond(message any) error` | Sends a direct response when supported.                              |

### `AuthenticationContext`

Authentication context is similar to request context but carries `ConnectionID` and has no `ResponseWriter`.

| Field          | Description                                                     |
| -------------- | --------------------------------------------------------------- |
| `Transport`    | Transport name.                                                 |
| `ConnectionID` | Transport-level connection identity, such as a UDP address key. |
| `ClientID`     | Current client ID before authentication completes.              |
| `Tick`         | Runtime tick when authentication is routed.                     |
| `ReceivedAt`   | Authentication timestamp.                                       |
| `Request`      | Parsed request message.                                         |
| `Runtime`      | Runtime routing the authentication attempt.                     |
| `Features`     | Runtime feature registry.                                       |

Method:

| Method                     | Purpose                                                              |
| -------------------------- | -------------------------------------------------------------------- |
| `Decode(target any) error` | Decodes `Request.Data` JSON into `target`; no-op when data is empty. |

## Response Writers

### `ResponseWriter`

```go
type ResponseWriter interface {
	Respond(message any) error
}
```

`RequestContext.Response` can be nil for non-transport tests or direct runtime calls.

### `ResponseWriterFunc`

`ResponseWriterFunc` adapts a function to `ResponseWriter`. A nil function returns `ErrResponsesUnsupported`.

## Error Values

These exported errors are intended for comparison with `errors.Is`:

| Error                                 | Raised when                                                                       |
| ------------------------------------- | --------------------------------------------------------------------------------- |
| `ErrMissingObjectType`                | An object or request lacks an object type.                                        |
| `ErrMissingObjectID`                  | An object or request lacks an object ID.                                          |
| `ErrObjectNotFound`                   | A target object or factory does not exist.                                        |
| `ErrObjectExists`                     | `CreateObject` is asked to create an existing object.                             |
| `ErrMissingAuthenticationHandler`     | Authentication is attempted with no registered handler.                           |
| `ErrAuthenticationHandlerExists`      | A second authentication object is registered.                                     |
| `ErrAuthenticationHandlerUnsupported` | Registered auth object does not implement `AuthenticationHandler`.                |
| `ErrAuthenticationRequired`           | An unauthenticated UDP client sends a regular request while auth is required.     |
| `ErrAuthenticatedClientIDInUse`       | A UDP client authenticates as an ID already used by another authenticated client. |
| `ErrMissingAuthenticationID`          | Authentication returns an empty ID.                                               |
| `ErrResponsesUnsupported`             | A response is attempted without a response writer.                                |
