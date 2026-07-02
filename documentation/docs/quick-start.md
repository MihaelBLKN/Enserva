# Quick Start

This page gets the included UDP host running and shows the smallest useful interaction path with Enserva's preferred binary wire protocol: buffer-backed wire packets. Legacy JSON datagrams still work and are shown at the end for compatibility and tooling.

!!! note "Language tabs"
    GoLang snippets are the canonical server/runtime examples for this repository. C# and Rust snippets show equivalent client-side, tooling, or SDK-shaped code for the same protocol concepts.

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
Use `go run . -udpAddr 0.0.0.0:9000` when you want clients on other machines to reach the server through any IPv4 interface.

## 2. Authenticate with a Wire Packet

When the sample objects are enabled, Enserva registers a `PlayerAuthenticator`. Send a `ClientHello` message inside a binary wire packet:

=== "GoLang"

    ```go
    package main

    import (
    	"fmt"
    	"log"
    	"net"
    	"time"

    	"Enserva/network"
    )

    func main() {
    	conn, err := net.Dial("udp", "127.0.0.1:9000")
    	if err != nil {
    		log.Fatal(err)
    	}
    	defer conn.Close()

    	hello, err := network.EncodeClientMessage(network.ClientHello{
    		ClientName:      "quickstart",
    		Token:           "",
    		ProtocolVersion: network.WireProtocolVersion,
    		Capabilities:    network.DefaultWireCapabilities(),
    		MaxPacketSize:   1200,
    	})
    	if err != nil {
    		log.Fatal(err)
    	}

    	packet, err := network.EncodePacket(1, []network.WireMessage{hello})
    	if err != nil {
    		log.Fatal(err)
    	}
    	if _, err := conn.Write(packet); err != nil {
    		log.Fatal(err)
    	}

    	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
    	buffer := make([]byte, 65535)
    	n, err := conn.Read(buffer)
    	if err != nil {
    		log.Fatal(err)
    	}

    	response, err := network.DecodePacket(buffer[:n])
    	if err != nil {
    		log.Fatal(err)
    	}
    	for _, message := range response.Messages {
    		decoded, err := network.DecodeServerMessage(message)
    		if err != nil {
    			log.Fatal(err)
    		}
    		fmt.Printf("%T %#v\n", decoded, decoded)
    	}
    }
    ```

=== "C#"

    ```csharp
    using System.Buffers.Binary;
    using System.Net.Sockets;
    using System.Text;

    static void WriteU16(List<byte> buffer, ushort value)
    {
        Span<byte> bytes = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(bytes, value);
        buffer.AddRange(bytes.ToArray());
    }

    static void WriteU32(List<byte> buffer, uint value)
    {
        Span<byte> bytes = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(bytes, value);
        buffer.AddRange(bytes.ToArray());
    }

    static void WriteU64(List<byte> buffer, ulong value)
    {
        Span<byte> bytes = stackalloc byte[8];
        BinaryPrimitives.WriteUInt64BigEndian(bytes, value);
        buffer.AddRange(bytes.ToArray());
    }

    static void WriteString(List<byte> buffer, string value)
    {
        byte[] text = Encoding.UTF8.GetBytes(value);
        WriteU16(buffer, (ushort)text.Length);
        buffer.AddRange(text);
    }

    var helloPayload = new List<byte>();
    WriteString(helloPayload, "quickstart");
    WriteString(helloPayload, "");
    helloPayload.Add(1);                       // protocol version
    WriteU64(helloPayload, 7);                  // capabilities: delta + reliable ordered/unordered
    WriteU32(helloPayload, 1200);               // max packet size

    var messages = new List<byte>();
    WriteU16(messages, 0x0001);                 // protocol.hello
    WriteU32(messages, (uint)helloPayload.Count);
    messages.AddRange(helloPayload);

    var packet = new List<byte>();
    WriteU16(packet, 0x4553);                   // magic "ES"
    packet.Add(1);                              // protocol version
    packet.Add(1);                              // message count
    WriteU32(packet, 0);                        // reserved
    WriteU64(packet, 1);                        // sequence
    WriteU64(packet, 0);                        // ack
    WriteU64(packet, 0);                        // ack_bits
    WriteU32(packet, (uint)messages.Count);
    packet.AddRange(messages);

    using var udp = new UdpClient();
    await udp.SendAsync(packet.ToArray(), packet.Count, "127.0.0.1", 9000);

    using UdpReceiveResult result = await udp.ReceiveAsync();
    Console.WriteLine($"received {result.Buffer.Length} bytes");
    ```

=== "Rust"

    ```rust
    use enserva_rust_client_example::{EnservaUdpClient, ServerMessage};

    let mut client = EnservaUdpClient::connect(
        "127.0.0.1:9000",
        "quickstart",
        "",
    )?;

    if let Some(ServerMessage::Welcome(welcome)) = client.receive_one()? {
        println!("authenticated as {}", welcome.authenticated_id);
    }
    ```

The sample authenticator creates a player object and responds with a binary `Welcome` message. With a fresh example server, the first authenticated client is usually assigned `player-1`.

## 3. Send Player Input

After authentication, send the built-in `PlayerInput` wire message from the same UDP client:

=== "GoLang"

    ```go
    input, err := network.EncodeClientMessage(network.PlayerInput{
    	Sequence: 2,
    	Tick:     0,
    	ObjectID: "player-1",
    	X:        1,
    	Y:        0,
    	Z:        0,
    })
    if err != nil {
    	log.Fatal(err)
    }

    packet, err := network.EncodePacket(2, []network.WireMessage{input})
    if err != nil {
    	log.Fatal(err)
    }
    if _, err := conn.Write(packet); err != nil {
    	log.Fatal(err)
    }
    ```

=== "C#"

    ```csharp
    using System.Buffers.Binary;
    using System.Net.Sockets;
    using System.Text;

    static void WriteU16(List<byte> buffer, ushort value)
    {
        Span<byte> bytes = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16BigEndian(bytes, value);
        buffer.AddRange(bytes.ToArray());
    }

    static void WriteU32(List<byte> buffer, uint value)
    {
        Span<byte> bytes = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(bytes, value);
        buffer.AddRange(bytes.ToArray());
    }

    static void WriteU64(List<byte> buffer, ulong value)
    {
        Span<byte> bytes = stackalloc byte[8];
        BinaryPrimitives.WriteUInt64BigEndian(bytes, value);
        buffer.AddRange(bytes.ToArray());
    }

    static void WriteF32(List<byte> buffer, float value)
    {
        Span<byte> bytes = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(bytes, BitConverter.SingleToUInt32Bits(value));
        buffer.AddRange(bytes.ToArray());
    }

    static void WriteString(List<byte> buffer, string value)
    {
        byte[] text = Encoding.UTF8.GetBytes(value);
        WriteU16(buffer, (ushort)text.Length);
        buffer.AddRange(text);
    }

    var inputPayload = new List<byte>();
    WriteString(inputPayload, "player-1");
    WriteU64(inputPayload, 2);
    WriteU64(inputPayload, 0);
    WriteF32(inputPayload, 1.0f);
    WriteF32(inputPayload, 0.0f);
    WriteF32(inputPayload, 0.0f);

    var messages = new List<byte>();
    WriteU16(messages, 0x0101);                 // engine.player_input
    WriteU32(messages, (uint)inputPayload.Count);
    messages.AddRange(inputPayload);

    var packet = new List<byte>();
    WriteU16(packet, 0x4553);
    packet.Add(1);
    packet.Add(1);
    WriteU32(packet, 0);
    WriteU64(packet, 2);
    WriteU64(packet, 0);
    WriteU64(packet, 0);
    WriteU32(packet, (uint)messages.Count);
    packet.AddRange(messages);

    using var udp = new UdpClient();
    await udp.SendAsync(packet.ToArray(), packet.Count, "127.0.0.1", 9000);
    ```

=== "Rust"

    ```rust
    use enserva_rust_client_example::EnservaUdpClient;

    let mut client = EnservaUdpClient::connect(
        "127.0.0.1:9000",
        "rust-client",
        "dev-token",
    )?;

    client.send_player_input("player-1", 2, 0, 1.0, 0.0, 0.0)?;
    ```

The player clamps each velocity axis to `[-1, 1]` and moves on each runtime tick according to its configured speed.

## 4. Receive Wire Snapshots

Wire clients receive `WorldSnapshot` messages wrapped in binary packets:

=== "GoLang"

    ```go
    _ = conn.SetReadDeadline(time.Now().Add(time.Second))
    n, err := conn.Read(buffer)
    if err != nil {
    	log.Fatal(err)
    }

    snapshotPacket, err := network.DecodePacket(buffer[:n])
    if err != nil {
    	log.Fatal(err)
    }
    for _, message := range snapshotPacket.Messages {
    	decoded, err := network.DecodeServerMessage(message)
    	if err != nil {
    		log.Fatal(err)
    	}
    	if snapshot, ok := decoded.(network.WorldSnapshot); ok {
    		fmt.Printf("tick=%d objects=%#v\n", snapshot.Tick, snapshot.Objects)
    	}
    }
    ```

=== "C#"

    ```csharp
    using System.Net.Sockets;

    using var udp = new UdpClient(9001);
    using UdpReceiveResult result = await udp.ReceiveAsync();

    // Decode the Enserva wire packet with your C# packet reader, then inspect
    // server messages for world snapshots.
    WirePacket packet = EnservaWire.DecodePacket(result.Buffer);
    foreach (WireMessage message in packet.Messages)
    {
        object decoded = EnservaWire.DecodeServerMessage(message);
        if (decoded is WorldSnapshot snapshot)
        {
            Console.WriteLine($"tick={snapshot.Tick} objects={snapshot.Objects.Count}");
        }
    }
    ```

=== "Rust"

    ```rust
    use enserva_rust_client_example::{EnservaUdpClient, ServerMessage};

    let mut client = EnservaUdpClient::connect(
        "127.0.0.1:9000",
        "rust-client",
        "dev-token",
    )?;

    if let Some(ServerMessage::WorldSnapshot(snapshot)) = client.receive_one()? {
        println!("tick={} object_types={}", snapshot.tick, snapshot.objects.len());
    }
    ```

!!! note
If an authentication object is registered, unauthenticated UDP clients do not receive snapshots and regular object requests are rejected.

## Legacy JSON Compatibility

JSON datagrams are still accepted for existing clients, diagnostics, and quick scripts. They use `network.RequestMessage` and receive JSON responses or snapshots unless the client first identifies itself with wire packets.

Authentication:

```json
{
  "type": "auth",
  "seq": 1,
  "data": {}
}
```

Player input:

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

## Minimal Server Code

=== "GoLang"

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

=== "C#"

    ```csharp
    // C# clients connect to the Go Enserva host over UDP.
    // Keep the authoritative server in Go, then start your C# client/tool separately.
    using var client = new UdpClient();
    await client.SendAsync(Array.Empty<byte>(), 0, "127.0.0.1", 9000);
    ```

=== "Rust"

    ```rust
    use std::net::UdpSocket;

    let socket = UdpSocket::bind("0.0.0.0:0")?;
    socket.send_to(&[], "127.0.0.1:9000")?;
    ```
