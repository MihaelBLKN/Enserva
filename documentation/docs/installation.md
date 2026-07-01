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
| `-udpPort`        | `9000`  | UDP port for the example server.                                 |
| `-tickRate`       | `128`   | Simulation ticks per second.                                     |
| `-snapshotRate`   | `20`    | Snapshot broadcasts per second.                                  |
| `-clientTimeout`  | `5s`    | UDP client inactivity timeout.                                   |
| `-maxUdpPacketSize` | `1200` | Maximum outbound UDP payload size in bytes.                      |
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

## Build the Documentation

From the repository root:

```bash
cd documentation
python -m pip install -r requirements.txt
mkdocs build
mkdocs serve
```

`mkdocs serve` starts a local documentation server, usually at `http://127.0.0.1:8000/`.
