# Installation

This guide installs Enserva from source and verifies that the project builds.

## Prerequisites

| Tool   | Required version       | Notes                                               |
| ------ | ---------------------- | --------------------------------------------------- |
| Go     | `1.26.3` or compatible | The module declares `go 1.26.3` in `go.mod`.        |
| Git    | Any current version    | Used to clone the repository.                       |
| Python | `3.10+` recommended    | Required only for building this documentation site. |

!!! note "Dependencies"
The Go module uses only the Go standard library. There is no `go.sum` file and no third-party Go dependency to download.

## Clone the Repository

```bash
git clone https://github.com/MihaelBLKN/Enserva.git
cd Enserva
```

## Build and Test

Use Go's normal module commands from the repository root:

```bash
go mod download
go build ./...
go test ./...
```

There is no Makefile, Taskfile, Mage target, Dockerfile, or other build wrapper.

## Run the Example Host

```bash
go run .
```

The host starts a UDP listener and registers the sample `netObjects` package by default.

Useful flags:

| Flag              | Default | Description                                                      |
| ----------------- | ------- | ---------------------------------------------------------------- |
| `-udpAddr`        | empty   | Full UDP listen address; overrides `-udpPort` when set.          |
| `-udpPort`        | `9000`  | UDP port for the example server.                                 |
| `-tickRate`       | `128`   | Simulation ticks per second.                                     |
| `-snapshotRate`   | `20`    | Snapshot broadcasts per second.                                  |
| `-clientTimeout`  | `5s`    | UDP client inactivity timeout.                                   |
| `-maxClients`     | `0`     | Maximum simultaneous UDP clients; `0` is unlimited.              |
| `-maxUdpPacketSize` | `1200` | Maximum outbound UDP payload size in bytes.                      |
| `-bandwidthBudget` | `false` | Enable per-client outbound byte budgeting.                       |
| `-clientBytesPerSecond` | `0` | Outbound byte budget per UDP client per second.                  |
| `-defaultSnapshotPriority` | `0` | Default priority for snapshot objects without explicit metadata. |
| `-reliableRetryInterval` | `100ms` | Retry interval for unacknowledged reliable wire messages. |
| `-reliableMaxAttempts` | `5` | Maximum send attempts for one reliable wire message. |
| `-reliableQueueLimit` | `64` | Maximum pending reliable messages per UDP client. |
| `-exampleObjects` | `true`  | Register the sample player, building, and authenticator objects. |
| `-debug`          | `false` | Serve the browser debug interface.                               |
| `-debugAddr`      | `:9100` | Debug interface HTTP address.                                    |

Example:

```bash
go run . -udpPort 9100 -tickRate 60 -snapshotRate 10
```

Bind to all IPv4 interfaces for a Linux or Windows Server deployment:

```bash
go run . -udpAddr 0.0.0.0:9000
```

Bind to all IPv6 interfaces:

```bash
go run . -udpAddr "[::]:9000"
```

## Cross-Platform Builds

The server code uses only the Go standard library and no shell wrapper, so the same source builds on Windows, Windows Server, and Linux distributions with a compatible Go toolchain.

Build for the current operating system:

```bash
go build -o enserva .
```

Cross-compile from a Unix-like shell:

```bash
GOOS=linux GOARCH=amd64 go build -o enserva .
GOOS=windows GOARCH=amd64 go build -o enserva.exe .
```

Cross-compile from PowerShell:

```powershell
$env:GOOS = "linux"; $env:GOARCH = "amd64"; go build -o enserva .
$env:GOOS = "windows"; $env:GOARCH = "amd64"; go build -o enserva.exe .
Remove-Item Env:\GOOS, Env:\GOARCH
```

## Build the Documentation

From the repository root:

```bash
cd documentation
python -m pip install -r requirements.txt
mkdocs build
mkdocs serve
```

`mkdocs serve` starts a local documentation server, usually at `http://127.0.0.1:8000/`.
