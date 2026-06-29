# Configuration

Enserva has two configuration layers:

- Programmatic runtime configuration through `network.Config`.
- CLI flags on the included example host in `main.go`.

Enserva does not define environment-variable or file-based configuration.

## Runtime Config

`network.Config` controls the core runtime and UDP transport:

| Field           | Type            | Default           | Description                                            |
| --------------- | --------------- | ----------------- | ------------------------------------------------------ |
| `UDPAddress`    | `string`        | `":9000"`         | Address passed to the UDP listener.                    |
| `TickRate`      | `int`           | `128`             | Number of simulation ticks per second.                 |
| `SnapshotRate`  | `int`           | `20`              | Number of snapshot broadcasts per second.              |
| `ClientTimeout` | `time.Duration` | `5 * time.Second` | Duration after which inactive UDP clients are removed. |

Use `network.DefaultConfig()` for defaults:

```go
config := network.DefaultConfig()
config.UDPAddress = ":9100"
config.TickRate = 60
config.SnapshotRate = 10

server := network.NewServer(config)
```

## Normalization Rules

`Config.Normalized()` applies defaults and guards invalid values:

| Input condition           | Result                                 |
| ------------------------- | -------------------------------------- |
| Empty `UDPAddress`        | Uses `":9000"`.                        |
| `TickRate <= 0`           | Uses `128`.                            |
| `SnapshotRate <= 0`       | Uses `20`.                             |
| `SnapshotRate > TickRate` | Clamps snapshot rate to the tick rate. |
| `ClientTimeout <= 0`      | Uses `5s`.                             |

!!! tip
    `NewServer` and `NewRuntime` both normalize the config before storing it.

## Derived Timing

| Method                   | Purpose                                        |
| ------------------------ | ---------------------------------------------- |
| `Config.TickInterval()`  | Returns `time.Second / TickRate`.              |
| `Config.SnapshotEvery()` | Returns the number of ticks between snapshots. |

Example:

```go
config := network.Config{TickRate: 120, SnapshotRate: 20}.Normalized()

tickInterval := config.TickInterval() // 8.333333ms
snapshotEvery := config.SnapshotEvery() // 6
```

## Example Host Flags

The root `main.go` exposes these flags:

| Flag              | Type       | Default | Description                                 |
| ----------------- | ---------- | ------- | ------------------------------------------- |
| `-udpPort`        | `int`      | `9000`  | UDP port. Converted to `Config.UDPAddress`. |
| `-tickRate`       | `int`      | `128`   | Simulation ticks per second.                |
| `-snapshotRate`   | `int`      | `20`    | Snapshot broadcasts per second.             |
| `-clientTimeout`  | `duration` | `5s`    | UDP client timeout.                         |
| `-exampleObjects` | `bool`     | `true`  | Register the sample `netObjects` package.   |

```bash
go run . -udpPort 9100 -tickRate 60 -snapshotRate 10 -clientTimeout 10s
```

## UDP Request Messages

Client requests are JSON UDP datagrams matching `network.RequestMessage`:

| JSON field   | Go field     | Required            | Description                                                                      |
| ------------ | ------------ | ------------------- | -------------------------------------------------------------------------------- |
| `type`       | `Type`       | Only for auth       | `auth` or `authentication` triggers authentication.                              |
| `seq`        | `Sequence`   | Recommended         | Monotonic sequence number; older duplicate sequences are ignored per UDP client. |
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

The sample `PlayerAuthenticator` does not inspect credentials; it creates a new player for each authentication attempt. Real applications should replace it with an object that validates credentials before returning an authenticated ID.

## Snapshot Messages

Snapshots match `network.SnapshotMessage`:

| JSON field | Description                                                |
| ---------- | ---------------------------------------------------------- |
| `type`     | Always `snapshot`.                                         |
| `clientId` | ID assigned to the receiving UDP client.                   |
| `tick`     | Runtime tick used for the snapshot.                        |
| `lastSeq`  | Last accepted client sequence number.                      |
| `objects`  | Nested map of object type to object ID to object snapshot. |

Objects can opt out of snapshots by implementing `SnapshotVisible() bool` and returning `false`.

## External Configuration

No stable external configuration format exists yet. If one is added later, this page should document file paths, schema, defaults, and precedence.
