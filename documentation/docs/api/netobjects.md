# Implementing Network Objects

Enserva includes a package named `Enserva/netObjects`, but that package is example application code. It shows how network objects can be implemented, registered, and tested.

The framework API lives in [`Enserva/network`](network.md). Your own project can create any package name you like for authoritative server objects.

!!! important
Do not treat the sample `Player`, `Building`, or `PlayerAuthenticator` behavior as Enserva framework behavior. They are examples of object implementations. The reusable contract is the set of interfaces and contexts from `network`.

## Basic Object Contract

Every object registered with Enserva must implement `network.Object`:

```go
type Object interface {
	ObjectType() string
	ObjectID() string
	Snapshot() any
}
```

| Method                | Purpose                                                                                                 |
| --------------------- | ------------------------------------------------------------------------------------------------------- |
| `ObjectType() string` | Returns the object category used for routing and snapshot grouping, such as `"player"` or `"building"`. |
| `ObjectID() string`   | Returns the unique object ID inside that object type.                                                   |
| `Snapshot() any`      | Returns the serializable state that should appear in snapshots.                                         |

The runtime trims object type and ID strings. Empty values are rejected.

## Optional Object Hooks

Objects can implement any of these methods. Enserva detects them through interfaces at runtime.

| Method                                                                   | Interface                       | When it runs                                                                                             |
| ------------------------------------------------------------------------ | ------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `OnInit(network.InitContext)`                                            | `network.InitHandler`           | Immediately after an object is registered with the runtime.                                              |
| `OnTick(network.TickContext)`                                            | `network.TickHandler`           | Every simulation tick after the runtime increments `Tick`.                                               |
| `OnFullTick(network.TickContext)`                                        | `network.FullTickHandler`       | Once per completed second of ticks, using `tick % TickRate == 0`.                                        |
| `OnRequest(network.RequestContext) error`                                | `network.RequestHandler`        | When a request targets an existing object with matching `objectType` and `objectId`.                     |
| `OnAuthenticationAttempt(network.AuthenticationContext) (string, error)` | `network.AuthenticationHandler` | When the transport receives an auth message and this object was registered as the authentication object. |
| `SnapshotVisible() bool`                                                 | `network.SnapshotVisibility`    | During snapshot generation. Return `false` for server-only objects.                                      |

!!! note
An object can implement only the hooks it needs. For example, a static object may only implement `ObjectType`, `ObjectID`, and `Snapshot`.

## Server-Side Factories

Factories are not object methods. They are server-side creation helpers:

```go
type ObjectFactory interface {
	CreateObject(network.RequestContext) (network.Object, error)
}
```

Most simple factories use `network.ObjectFactoryFunc`:

```go
func ProjectileFactory(ctx network.RequestContext) (network.Object, error) {
	return &Projectile{ID: ctx.Request.ObjectID}, nil
}

if err := server.RegisterFactory("projectile", network.ObjectFactoryFunc(ProjectileFactory)); err != nil {
	return err
}
```

Client requests do not call factories. To create an object through a factory, server code must call:

```go
object, err := server.CreateObject("projectile", "projectile-1")
```

## Registration Methods

Use these `network.Server` or `network.Runtime` methods to make objects available:

| Method                                                                    | Use                                                                  |
| ------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| `RegisterObject(object network.Object) error`                             | Register an object that already exists.                              |
| `RegisterAuthenticationObject(object network.Object) error`               | Register an object and bind it as the single authentication handler. |
| `RegisterFactory(objectType string, factory network.ObjectFactory) error` | Register a server-side creation helper.                              |
| `CreateObject(objectType, objectID string) (network.Object, error)`       | Create an object through a registered factory.                       |
| `RemoveObject(objectType, objectID string)`                               | Remove an object from the runtime.                                   |
| `GetObject(objectType, objectID string) (network.Object, bool)`           | Look up a registered object.                                         |

## Minimal Object Example

```go
package world

import "Enserva/network"

type Door struct {
	ID         string `json:"id"`
	Open       bool   `json:"open"`
	LastClient string `json:"lastClient,omitempty"`
}

func (door *Door) ObjectType() string {
	return "door"
}

func (door *Door) ObjectID() string {
	return door.ID
}

func (door *Door) Snapshot() any {
	return *door
}

func (door *Door) OnRequest(ctx network.RequestContext) error {
	switch ctx.Request.Action {
	case "open":
		door.Open = true
	case "close":
		door.Open = false
	}

	door.LastClient = ctx.ClientID
	return nil
}
```

Register it:

```go
door := &world.Door{ID: "door-1"}
if err := server.RegisterObject(door); err != nil {
	return err
}
```

## Init Hook Example

Use `OnInit` for setup that needs the runtime after an object is registered. Interest management is a common example:

```go
func (player *Player) OnInit(ctx network.InitContext) {
	ctx.Runtime().Features().EnableInterestManagement(
		network.PlayerInterest2D(player, "x", "y", 500),
	)
}
```

## Tick Hook Example

```go
func (projectile *Projectile) OnTick(ctx network.TickContext) {
	projectile.X += projectile.VelocityX * ctx.DeltaSeconds
	projectile.Y += projectile.VelocityY * ctx.DeltaSeconds
}
```

`TickContext` includes:

| Field          | Use                                 |
| -------------- | ----------------------------------- |
| `Tick`         | Current runtime tick.               |
| `Delta`        | Tick duration as `time.Duration`.   |
| `DeltaSeconds` | Tick duration as `float64` seconds. |
| `Runtime`      | Runtime invoking the hook.          |
| `Features`     | Runtime feature registry.           |

## Request Hook Basics

`OnRequest` receives a `network.RequestContext`:

```go
func (object *Thing) OnRequest(ctx network.RequestContext) error {
	var payload struct {
		Value int `json:"value"`
	}
	if err := ctx.Decode(&payload); err != nil {
		return err
	}

	return nil
}
```

Useful fields and methods:

| Field or method          | Use                                                    |
| ------------------------ | ------------------------------------------------------ |
| `ctx.ClientID`           | Authenticated or transport-level client identity.      |
| `ctx.Request.ObjectType` | Target object type.                                    |
| `ctx.Request.ObjectID`   | Target object ID.                                      |
| `ctx.Request.Action`     | Object-defined action name.                            |
| `ctx.Decode(&target)`    | Decode `ctx.Request.Data` JSON.                        |
| `ctx.Respond(message)`   | Send a direct response when the transport supports it. |
| `ctx.Runtime`            | Access the runtime routing the request.                |
| `ctx.Features`           | Access runtime feature configuration.                  |

## Authentication Object Basics

Authentication is handled by one normal object that implements `OnAuthenticationAttempt`.

```go
func (auth *Authenticator) OnAuthenticationAttempt(ctx network.AuthenticationContext) (string, error) {
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

	return "player-" + accountID, nil
}
```

Register it:

```go
if err := server.RegisterAuthenticationObject(authenticator); err != nil {
	return err
}
```

Only one authentication object can be registered at a time. If an authentication object exists, unauthenticated UDP clients cannot receive snapshots or send normal object requests.

## Hiding Server-Only Objects

Implement `SnapshotVisible` when an object should exist in the runtime but not be sent to clients:

```go
func (auth *Authenticator) SnapshotVisible() bool {
	return false
}
```

This is useful for authentication handlers and other server-only coordination objects.

## Included Example Package

The repository's `Enserva/netObjects` package includes:

| Example                                  | Demonstrates                                                       |
| ---------------------------------------- | ------------------------------------------------------------------ |
| `Player`                                 | An object with request, tick, and full-tick hooks.                 |
| `Building`                               | An object with init, request, and full-tick hooks.                 |
| `PlayerAuthenticator`                    | A hidden authentication object that creates a server-owned player. |
| `Register(server *network.Server) error` | One way to group object/factory registration for an app package.   |

Read these files as implementation examples only. The object names, actions, payload fields, and gameplay behavior are not part of the Enserva core API.
