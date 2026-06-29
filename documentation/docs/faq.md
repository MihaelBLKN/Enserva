# FAQ

## Is Enserva a full game server?

No. Enserva is an early game-server networking API. It provides a small authoritative runtime, UDP transport, object hooks, snapshots, and sample objects.

## Can clients create objects by sending requests?

No. Client requests route only to objects that already exist. Factories are called by server-side `CreateObject` calls.

## How many authentication handlers can I register?

One. Registering a second authentication object returns `ErrAuthenticationHandlerExists`. Removing the authentication object unbinds the handler.

## Are snapshots sent to unauthenticated clients?

Not when an authentication object is registered. The UDP server skips unauthenticated clients until authentication succeeds.

## Does the sample authenticator validate credentials?

No. `netObjects.PlayerAuthenticator` creates a new player for each authentication attempt. Replace it for production use.

## Can objects be hidden from snapshots?

Yes. Implement `SnapshotVisible() bool` and return `false`.

## Are object hooks called concurrently?

The runtime serializes calls to tick, request, authentication, and snapshot hooks with `hooksMu`. Long-running hooks can delay other runtime work.

## What transports are supported?

Enserva ships with a UDP transport. Runtime contexts include a `Transport` field, so other transports can be added later.

## Is there a stable wire protocol?

The JSON shapes are documented in [Configuration](configuration.md), but the project is still early. Treat the protocol as provisional until releases document compatibility guarantees.
