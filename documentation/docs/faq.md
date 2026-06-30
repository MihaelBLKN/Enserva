# FAQ

## Is Enserva a full game server?

Enserva is a server runtime, not a complete game backend product. It gives you authoritative objects, ticks, UDP transport, snapshots, interest filtering, scenes, authentication hooks, and debug tooling. You still own game rules, persistence, matchmaking, deployment, and client protocol design.

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

## Can clients switch scenes directly?

Clients can request a scene switch, but the server decides. Standard `scene.switch` requests route to the target object's `OnSceneSwitchRequest`, and scene state is only mutated when the handler returns an allowed decision.

## Are scenes the same as loading maps?

No. Scenes are runtime visibility and membership state. Your game code still owns map loading, spawn rules, persistence, and any gameplay consequences of entering or leaving a scene.

## Are object hooks called concurrently?

The runtime serializes calls to tick, request, authentication, and snapshot hooks with `hooksMu`. Long-running hooks can delay other runtime work.

## What transports are supported?

Enserva ships with a UDP transport. Runtime contexts include a `Transport` field, so other transports can be added later.

## Is there a stable wire protocol?

The binary packet format is the preferred client protocol and is documented in [Wire Protocol](api/wire-protocol.md). Legacy JSON request shapes are documented in [Configuration](configuration.md) for compatibility and tooling. The project is still early, so treat both protocols as provisional until releases document compatibility guarantees.
