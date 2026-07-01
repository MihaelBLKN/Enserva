# Configuration

Enserva has two configuration layers:

- Programmatic runtime configuration through `network.Config`.
- CLI flags on the included example host in `main.go`.

Enserva does not define environment-variable or file-based configuration.

## Runtime Config

`network.Config` controls the core runtime and UDP transport:

| Field                  | Type            | Default           | Description                                                              |
| ---------------------- | --------------- | ----------------- | ------------------------------------------------------------------------ |
| `UDPAddress`           | `string`        | `":9000"`         | Address passed to the UDP listener.                                      |
| `TickRate`             | `int`           | `128`             | Number of simulation ticks per second.                                   |
| `SnapshotRate`         | `int`           | `20`              | Number of snapshot broadcasts per second.                                |
| `EnableDeltaSnapshots` | `bool`          | `false`           | Enables per-client delta snapshots after each client's first full state. |
| `SupportedWireCapabilities` | `uint64`    | `DefaultWireCapabilities()` with delta masked off unless enabled | Optional server-side capability mask used during wire hello/welcome negotiation. |
| `FullSnapshotInterval` | `int`           | `64`              | Maximum emitted snapshots in a delta baseline cycle, including the full. |
| `ClientTimeout`        | `time.Duration` | `5 * time.Second` | Duration after which inactive UDP clients are removed.                   |
| `MaxClients`           | `int`           | `0`               | Maximum simultaneous UDP clients. `0` allows unlimited clients.          |
| `MaxUDPPacketSize`     | `int`           | `1200`            | Maximum serialized outbound UDP payload size in bytes.                   |
| `EnableBandwidthBudget` | `bool`         | `false`           | Enables per-client outbound byte budgeting for UDP traffic.              |
| `ClientBytesPerSecond` | `int`           | `0`               | Token-bucket refill rate and capacity for each UDP client when budgeting is enabled. |
| `DefaultSnapshotPriority` | `OutboundPriority` | `OutboundPriorityNormal` | Priority used for snapshot objects that do not implement `SnapshotPriorityProvider`. |
| `ReliableRetryInterval` | `time.Duration` | `100 * time.Millisecond` | How long UDP waits before retransmitting an unacknowledged reliable wire message. |
| `ReliableMaxAttempts`  | `int`           | `5`               | Maximum send attempts for one reliable wire message, including the first send. |
| `ReliableQueueLimit`   | `int`           | `64`              | Maximum pending outgoing reliable messages per UDP client.               |
| `MaxInputFutureTicks`  | `uint64`        | `8`               | Largest accepted client input tick ahead of the current runtime tick.    |
| `MaxInputPastTicks`    | `uint64`        | `2`               | Largest accepted client input tick behind the current runtime tick.      |
| `InputBufferLimit`     | `int`           | `256`             | Maximum buffered inputs retained per client.                             |
| `DebugEnabled`         | `bool`          | `false`           | Starts the browser debug UI when the server starts.                      |
| `DebugAddress`         | `string`        | `":9100"`         | Address passed to the debug HTTP listener.                               |

Use `network.DefaultConfig()` for defaults:

=== "GoLang"

    ```go
    config := network.DefaultConfig()
    config.UDPAddress = ":9100"
    config.TickRate = 60
    config.SnapshotRate = 10
    config.EnableDeltaSnapshots = true
    config.FullSnapshotInterval = 32
    config.MaxClients = 64
    config.MaxUDPPacketSize = 1200
    config.EnableBandwidthBudget = true
    config.ClientBytesPerSecond = 24_000
    config.DefaultSnapshotPriority = network.OutboundPriorityNormal
    config.ReliableRetryInterval = 100 * time.Millisecond
    config.ReliableMaxAttempts = 5
    config.ReliableQueueLimit = 64
    config.MaxInputFutureTicks = 8
    config.MaxInputPastTicks = 2
    config.InputBufferLimit = 256

    server := network.NewServer(config)
    ```

=== "C#"

    ```csharp
    // Conceptual C# host API shape if you wrap Enserva from another runtime.
    var config = EnservaConfig.Default();
    var server = new EnservaServer(config);

    server.RegisterFactory("player", PlayerFactory.Create);
    server.RegisterAuthenticationObject(new PlayerAuthenticator("default"));

    await server.ListenAndServeAsync();
    ```

## Normalization Rules

`Config.Normalized()` applies defaults and guards invalid values:

| Input condition              | Result                                      |
| ---------------------------- | ------------------------------------------- |
| Empty `UDPAddress`           | Uses `":9000"`.                             |
| `TickRate <= 0`              | Uses `128`.                                 |
| `SnapshotRate <= 0`          | Uses `20`.                                  |
| `SnapshotRate > TickRate`    | Clamps snapshot rate to the tick rate.      |
| `FullSnapshotInterval <= 0`  | Uses `64`.                                  |
| `SupportedWireCapabilities == 0` | Uses `DefaultWireCapabilities()`.       |
| `EnableDeltaSnapshots == false` | Removes `WireCapabilityDeltaSnapshots` from supported capabilities. |
| `ClientTimeout <= 0`         | Uses `5s`.                                  |
| `MaxClients <= 0`            | Allows unlimited UDP clients.              |
| `MaxUDPPacketSize <= 0`      | Uses `1200`.                                |
| `MaxUDPPacketSize > 65507`   | Clamps to the maximum UDP payload size.     |
| `EnableBandwidthBudget == true` and `ClientBytesPerSecond <= 0` | Disables bandwidth budgeting. |
| `ReliableRetryInterval <= 0` | Uses `100ms`.                               |
| `ReliableMaxAttempts <= 0`   | Uses `5`.                                   |
| `ReliableQueueLimit <= 0`    | Uses `64`.                                  |
| `MaxInputFutureTicks == 0`   | Uses `8`.                                   |
| `MaxInputPastTicks == 0`     | Uses `2`.                                   |
| `InputBufferLimit <= 0`      | Uses `256`.                                 |
| Empty `DebugAddress`         | Uses `":9100"`.                             |

!!! tip
`NewServer` and `NewRuntime` both normalize the config before storing it.

## Derived Timing

| Method                         | Purpose                                                                    |
| ------------------------------ | -------------------------------------------------------------------------- |
| `Config.TickInterval()`        | Returns `time.Second / TickRate`.                                          |
| `Config.SnapshotEvery()`       | Returns the number of ticks between snapshots.                             |
| `Config.FullSnapshotEvery()`   | Returns the normalized full snapshot interval for delta baseline cycling.  |

Example:

=== "GoLang"

    ```go
    config := network.Config{TickRate: 120, SnapshotRate: 20}.Normalized()

    tickInterval := config.TickInterval() // 8.333333ms
    snapshotEvery := config.SnapshotEvery() // 6
    ```

=== "C#"

    ```csharp
    var config = new EnservaConfig
    {
        TickRate = 120,
        SnapshotRate = 20,
    }.Normalize();

    TimeSpan tickInterval = config.TickInterval;
    int snapshotEvery = config.SnapshotEvery;
    ```

## Example Host Flags

The root `main.go` exposes these flags:

| Flag                    | Type       | Default | Description                                      |
| ----------------------- | ---------- | ------- | ------------------------------------------------ |
| `-udpAddr`              | `string`   | empty   | Full UDP listen address. Overrides `-udpPort` when set. |
| `-udpPort`              | `int`      | `9000`  | UDP port. Converted to `Config.UDPAddress`.      |
| `-tickRate`             | `int`      | `128`   | Simulation ticks per second.                     |
| `-snapshotRate`         | `int`      | `20`    | Snapshot broadcasts per second.                  |
| `-deltaSnapshots`       | `bool`     | `false` | Enable per-client delta snapshots.               |
| `-fullSnapshotInterval` | `int`      | `64`    | Maximum emitted snapshots per full/delta cycle.  |
| `-clientTimeout`        | `duration` | `5s`    | UDP client timeout.                              |
| `-maxClients`           | `int`      | `0`     | Maximum simultaneous UDP clients; `0` is unlimited. |
| `-maxUdpPacketSize`     | `int`      | `1200`  | Maximum outbound UDP payload size in bytes.      |
| `-bandwidthBudget`      | `bool`     | `false` | Enable per-client outbound byte budgeting.       |
| `-clientBytesPerSecond` | `int`      | `0`     | Outbound byte budget per UDP client per second.  |
| `-defaultSnapshotPriority` | `int`   | `0`     | Default priority for snapshot objects without explicit metadata. |
| `-reliableRetryInterval` | `duration` | `100ms` | Retry interval for unacknowledged reliable wire messages. |
| `-reliableMaxAttempts`  | `int`      | `5`     | Maximum send attempts for one reliable wire message. |
| `-reliableQueueLimit`   | `int`      | `64`    | Maximum pending reliable messages per UDP client. |
| `-maxInputFutureTicks`  | `uint64`   | `8`     | Maximum accepted input ticks ahead of the current runtime tick. |
| `-maxInputPastTicks`    | `uint64`   | `2`     | Maximum accepted input ticks behind the current runtime tick. |
| `-inputBufferLimit`     | `int`      | `256`   | Maximum buffered inputs per client.              |
| `-exampleObjects`       | `bool`     | `true`  | Register the sample `netObjects` package.        |
| `-debug`                | `bool`     | `false` | Serve the browser debug interface.               |
| `-debugAddr`            | `string`   | `:9100` | Debug interface HTTP address.                    |

```bash
go run . -udpAddr 0.0.0.0:9100 -tickRate 60 -snapshotRate 10 -deltaSnapshots -fullSnapshotInterval 32 -clientTimeout 10s -maxClients 64 -maxUdpPacketSize 1200 -bandwidthBudget -clientBytesPerSecond 24000 -reliableRetryInterval 100ms -reliableMaxAttempts 5 -reliableQueueLimit 64 -maxInputFutureTicks 8 -maxInputPastTicks 2 -inputBufferLimit 256
```

`-udpPort` preserves the simple default path and creates a hostless address such as `:9000`. Use `-udpAddr` for deployments that need an explicit interface, for example `127.0.0.1:9000`, `0.0.0.0:9000`, or `[::]:9000`.

Launch the browser debug interface while the UDP server runs:

```bash
go run . -debug
```

The default debug URL is `http://localhost:9100`. The interface polls `/debug/state` and displays normalized config, runtime ticks, registered factories, authentication state, interest-management data, UDP clients, transport counters, and all registered object snapshots including objects hidden from normal client snapshots.

## Client Limit

`Config.MaxClients` caps the number of simultaneous UDP client addresses tracked by the built-in transport, similar to a max-player setting for a game server. Existing clients continue to be accepted, but new UDP addresses are dropped once the cap is reached. Set it to `0` or any negative value to allow unlimited clients.

## Outbound UDP Packet Limit

`Config.MaxUDPPacketSize` limits the serialized UDP payload sent by Enserva, not including IP or UDP headers. The default is 1200 bytes to avoid common internet path MTU fragmentation. You may raise it for controlled networks, but values above the maximum UDP payload size are clamped during normalization.

The limit applies to both binary wire packets and legacy JSON packets. Immediate responses are checked after serialization and before `WriteToUDP`; if they exceed the limit, the response is dropped and `ErrUDPPacketTooLarge` is returned to the response caller. Snapshots are checked the same way and skipped for that client on that tick when oversized.

Oversized outbound drops are logged and counted in the debug state as `udp.counters.oversizedOutboundPacketsDropped`.

## Outbound Bandwidth Budget

`Config.EnableBandwidthBudget` turns on a per-client UDP token bucket. `ClientBytesPerSecond` is both the refill rate and the maximum burst capacity. When a client does not have enough tokens for an outbound packet, immediate lower-level responses are dropped and deferrable traffic such as snapshots or reliable retransmits waits for a later refill.

Snapshot objects can implement `SnapshotPriorityProvider` to return a generic `OutboundPriority`. When a snapshot is over the negotiated MTU or remaining budget, the UDP transport re-encodes the snapshot while omitting lower-priority objects before higher-priority objects. Objects without explicit metadata use `DefaultSnapshotPriority`.

Built-in protocol responses such as authentication, errors, disconnects, and pongs use high or essential priorities. Priority only orders traffic competing for a client's budget; it does not make a message reliable or game-authoritative by itself.

Budget counters are exposed in debug state as `udp.counters.bandwidthBudgetDrops`, `bandwidthBudgetDeferrals`, and `outboundBytesSent`, with per-client `bytesSent`, `bandwidthBudgetDrops`, and `bandwidthBudgetDeferrals`.

## Reliable UDP Delivery

Reliable delivery applies only to binary wire messages that opt in with `DeliveryReliableOrdered`, `DeliveryReliableUnordered`, or `network.DeliverReliableOrdered` / `network.DeliverReliableUnordered`. Unwrapped messages, legacy JSON datagrams, player input, and snapshot traffic remain unreliable.

The server keeps outgoing reliable messages in a per-client retry queue until the client acknowledges a packet sequence that carried the message. `ReliableRetryInterval` controls retry timing, `ReliableMaxAttempts` controls when an unacknowledged message is dropped, and `ReliableQueueLimit` bounds memory per client. Reliable counters are exposed under `udp.counters` as queued messages, retransmits, drops, and ack removals.

## Client Input Buffering

The runtime can buffer tick-aligned client input before game code consumes it. `MaxInputFutureTicks` rejects inputs too far ahead of the current runtime tick, `MaxInputPastTicks` rejects stale inputs, and `InputBufferLimit` bounds retained inputs per client. Accepted inputs are ordered by target tick and input sequence.

Game code reads inputs with `Runtime.ConsumeClientInputs`, `ConsumeClientInputsForTick`, `ConsumeClientInputsForObject`, or `ConsumeClientInputsForObjectAtTick`, usually from an object's `OnTick`. These APIs return generic `ClientInput` envelopes and do not apply movement, combat, inventory, or other game behavior.

Input-buffer counters are exposed in debug state as `runtime.inputBuffer.buffered`, `consumed`, `staleRejected`, `futureRejected`, and `dropped`.

## Delta Snapshots

By default, Enserva preserves the original behavior and sends full snapshots. When `Config.EnableDeltaSnapshots` is true, the UDP transport tracks a baseline per client connection. The first eligible snapshot is always full. Later snapshots contain only objects that spawned, changed, or despawned relative to that client's previous visible snapshot.

`FullSnapshotInterval` bounds the baseline cycle. With the default interval of `64`, one full snapshot is followed by up to 63 delta snapshots before the next full snapshot refreshes the baseline. Set it to `1` to force every snapshot to be full while still leaving the delta machinery enabled.

Deltas are calculated after scene filtering, snapshot visibility, and interest management. Invisible objects are not included in spawned or changed data. Objects that were previously visible and are now removed or no longer visible appear in `despawned`.

The server also forces a full snapshot when no baseline exists, after authentication changes a client ID, after a client switches from JSON to wire packets, after a scene-switch request, or when an inactive UDP client is removed and later reconnects.

Legacy JSON clients receive `DeltaSnapshotMessage` envelopes with `type: "snapshot.delta"`. Wire clients receive `WorldDeltaSnapshot` (`engine.delta_snapshot`). Debug counters expose total snapshots plus `fullSnapshotsSent` and `deltaSnapshotsSent`.

## Wire Capability Negotiation

`SupportedWireCapabilities` controls the optional protocol features the server is willing to negotiate with a wire client. When it is left as `0`, normalization uses `network.DefaultWireCapabilities()`, currently delta snapshots, reliable ordered delivery, and reliable unordered delivery. If `EnableDeltaSnapshots` is false, normalization clears the delta-snapshot bit even when the default or custom mask includes it.

During a `ClientHello`, the server intersects the client's requested capabilities with the normalized server mask and returns the result in `Welcome`. The negotiated maximum packet size is the lower of the client's `MaxPacketSize`, when supplied, and `Config.MaxUDPPacketSize`.

## UDP Wire Packets

The UDP transport's primary protocol is the binary wire packet format. These buffer-backed packets start with the `ES` magic value and carry one or more registered messages, sender sequence state, acknowledgement fields, and a bounded payload buffer.

The built-in registry includes hello, welcome, ping, pong, error, disconnect, object request, player input, world snapshot, aggregate delta snapshot, and entity delta message schemas. Register game-specific messages in the `0x1000-0xffff` range for gameplay traffic instead of inventing ad hoc JSON envelopes.

Use this protocol for new clients, multiplayer hot paths, custom gameplay messages, and snapshot handling. See [Wire Protocol](api/wire-protocol.md) for packet layout, message IDs, and registration examples.

## Legacy JSON Request Messages

Legacy clients and tooling may still send JSON UDP datagrams matching `network.RequestMessage`:

| JSON field   | Go field     | Required            | Description                                                                      |
| ------------ | ------------ | ------------------- | -------------------------------------------------------------------------------- |
| `type`       | `Type`       | Only for auth       | `auth` or `authentication` triggers authentication.                              |
| `seq`        | `Sequence`   | Recommended         | Monotonic sequence number. Older duplicate sequences are ignored per UDP client. |
| `objectType` | `ObjectType` | For object requests | Target object type.                                                              |
| `objectId`   | `ObjectID`   | For object requests | Target object ID.                                                                |
| `action`     | `Action`     | Object-specific     | Action name interpreted by the object.                                           |
| `data`       | `Data`       | Object-specific     | Raw JSON payload decoded by the target object.                                   |

!!! warning "Clients cannot create missing objects"
`Runtime.HandleRequest` routes only to objects that already exist. Registered factories are used by server-side calls to `CreateObject`, not by client requests.

## Authentication Messages

Authentication uses the same request envelope with `type` set to `auth` or `authentication`:

```json
{
  "type": "auth",
  "seq": 1,
  "data": {
    "token": "client-token"
  }
}
```

The sample `PlayerAuthenticator` does not inspect credentials. It creates a new player for each authentication attempt. Real applications should replace it with an object that validates credentials before returning an authenticated ID.

## Legacy JSON Snapshot Messages

Clients that have not sent wire packets receive snapshots as `network.SnapshotMessage` JSON:

| JSON field | Description                                                |
| ---------- | ---------------------------------------------------------- |
| `type`     | Always `snapshot`.                                         |
| `clientId` | ID assigned to the receiving UDP client.                   |
| `tick`     | Runtime tick used for the snapshot.                        |
| `lastSeq`  | Last accepted client sequence number.                      |
| `objects`  | Nested map of object type to object ID to object snapshot. |

Objects can opt out of snapshots by implementing `SnapshotVisible() bool` and returning `false`.

When delta snapshots are enabled, legacy JSON clients may also receive `network.DeltaSnapshotMessage`:

| JSON field     | Description                                                              |
| -------------- | ------------------------------------------------------------------------ |
| `type`         | Always `snapshot.delta`.                                                 |
| `clientId`     | ID assigned to the receiving UDP client.                                 |
| `tick`         | Runtime tick used for the delta.                                         |
| `lastSeq`      | Last accepted client sequence number.                                    |
| `baselineTick` | Tick of the previous full or delta snapshot used as the baseline.        |
| `spawned`      | Newly visible objects grouped by object type and ID.                     |
| `changed`      | Previously visible objects whose canonical snapshot value changed.       |
| `despawned`    | Object type and ID pairs that disappeared or became invisible to client. |

## External Configuration

No stable external configuration format exists yet. If one is added later, this page should document file paths, schema, defaults, and precedence.
