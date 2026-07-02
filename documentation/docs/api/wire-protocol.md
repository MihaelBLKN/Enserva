# Wire Protocol

Enserva's UDP transport uses binary wire packets as the primary client protocol. New clients should send wire packets, use the built-in protocol and engine messages where they fit, and register typed game messages for project-specific traffic. Legacy JSON request datagrams are still accepted for compatibility, simple scripts, and development tooling.

## Packet Format

All integer fields are big endian.

| Field              | Type     | Notes                                                                                               |
| ------------------ | -------- | --------------------------------------------------------------------------------------------------- |
| `magic`            | `uint16` | ASCII `ES`, value `0x4553`.                                                                         |
| `protocol_version` | `uint8`  | Current version is `1`.                                                                             |
| `message_count`    | `uint8`  | Number of framed messages in this packet.                                                           |
| `reserved`         | `uint32` | Reserved for future flags. Send as zero.                                                            |
| `sequence`         | `uint64` | Packet sequence number from the sender.                                                             |
| `ack`              | `uint64` | Latest peer packet sequence received by the sender.                                                 |
| `ack_bits`         | `uint64` | Bit `0` acknowledges `ack - 1`, bit `1` acknowledges `ack - 2`, and so on through 64 prior packets. |
| `payload_length`   | `uint32` | Total bytes after the packet header.                                                                |

Each message then has:

| Field            | Type     | Notes                                                              |
| ---------------- | -------- | ------------------------------------------------------------------ |
| `message_type`   | `uint16` | Stable registered message ID, or `0x0007` for a reliable envelope. |
| `payload_length` | `uint32` | Message payload length.                                            |
| `payload`        | bytes    | Encoded by the registered message definition.                      |

Oversized packets, malformed lengths, and unsupported protocol versions are rejected before gameplay dispatch.

## Capability Negotiation

`protocol.hello` and `protocol.welcome` now carry optional negotiation fields after the legacy `client_name` / `token` or `client_id` / `authenticated_id` strings.

| Field              | Type     | Notes                                                                                                                                                          |
| ------------------ | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `protocol_version` | `uint8`  | Optional protocol version requested by the client and echoed by the server when present. Older clients can omit it and keep using the legacy two-string shape. |
| `capabilities`     | `uint64` | Bitset of optional protocol features. Unknown future bits are ignored by older peers.                                                                          |
| `max_packet_size`  | `uint32` | Optional per-client packet size limit. The server still applies its own configured maximum and uses the lower of the two values.                               |

Capability bits currently defined by the built-in server:

| Bit | Capability | Behavior |
| --- | --- | --- |
| `0` | Delta snapshots | Allows negotiated clients to receive `engine.delta_snapshot` packets when delta snapshots are enabled. |
| `1` | Reliable ordered delivery | Allows negotiated clients to send and receive `DeliveryReliableOrdered` envelopes. |
| `2` | Reliable unordered delivery | Allows negotiated clients to send and receive `DeliveryReliableUnordered` envelopes. |

The server keeps legacy clients functional by treating omitted negotiation fields as defaults. Once a client sends negotiation fields, the server only enables the intersection of the requested and supported capabilities. Unsupported protocol versions are rejected with the wire error `unsupported wire protocol version`.

## Delivery Classes

Wire messages are unreliable by default. This preserves the normal UDP behavior used by high-rate traffic such as snapshots and player input. A message becomes reliable only when it is explicitly wrapped with delivery metadata or when its registered `WireMessageDefinition.Delivery` is set to a reliable class.

| Delivery class              | Behavior                                                                                                                                                                                                    |
| --------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `DeliveryUnreliable`        | Original packet behavior. The message is dispatched if the packet is accepted, but it is not retried and duplicate reliable IDs are not tracked.                                                            |
| `DeliveryReliableOrdered`   | The UDP transport retries until a packet carrying the message is acknowledged or the configured attempt limit is reached. Inbound messages are dispatched to runtime handlers in reliable message ID order. |
| `DeliveryReliableUnordered` | The UDP transport retries until acknowledgement but dispatches accepted inbound messages immediately, suppressing duplicate reliable IDs.                                                                   |

Reliable messages use the normal message header with `message_type = 0x0007`. The reliable envelope payload is:

| Field                  | Type     | Notes                                                  |
| ---------------------- | -------- | ------------------------------------------------------ |
| `delivery_class`       | `uint8`  | `1` for reliable ordered, `2` for reliable unordered.  |
| `reliable_id`          | `uint64` | Monotonic non-zero ID in the sender's reliable stream. |
| `inner_message_type`   | `uint16` | Registered message ID for the wrapped payload.         |
| `inner_payload_length` | `uint32` | Wrapped payload length.                                |
| `inner_payload`        | bytes    | Encoded by the inner message definition.               |

Packet `ack` and `ack_bits` still acknowledge packet sequence numbers, not reliable IDs. The server removes an outgoing reliable message from its retry queue when any packet that carried it is acknowledged. Retransmits reuse the same reliable ID inside a new packet sequence, so receivers suppress duplicates by reliable ID.

Reliable ordered IDs are expected to start at `1` for a connection and increase by one for that ordered stream. If ID `3` arrives before ID `2`, dispatch waits until `2` is received. Reliable unordered delivery does not wait for gaps.

## Outbound Priority And Budgeting

Outbound priority is server-side transport metadata. It is not encoded as a new packet field and clients do not need to parse it. The UDP server uses priority only when `Config.EnableBandwidthBudget` is enabled or when a snapshot must be trimmed to fit the remaining budget and negotiated packet size.

Built-in priority constants are:

| Priority | Use |
| --- | --- |
| `OutboundPriorityLow` | Disposable or easily refreshed traffic. |
| `OutboundPriorityNormal` | Default application and snapshot traffic. |
| `OutboundPriorityHigh` | Important responses or state. |
| `OutboundPriorityEssential` | Protocol-critical responses such as authentication failures or disconnects. |

Servers can wrap one-off responses with `network.Prioritize`, `PrioritizeLow`, `PrioritizeHigh`, or `PrioritizeEssential`. Snapshot objects can implement `SnapshotPriority() network.OutboundPriority`; objects without that method use `Config.DefaultSnapshotPriority`.

When a snapshot is over budget, the server omits lower-priority object updates before higher-priority updates and stores the filtered snapshot as the client's baseline. Omitted objects can appear in later full or delta snapshots after the budget refills.

## Message ID Ranges

| Range           | Owner                                                                                                                                  |
| --------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `0x0000-0x00ff` | Protocol/system messages such as hello, welcome, ping, pong, error, and disconnect.                                                    |
| `0x0100-0x0fff` | Enserva engine messages such as the generic object request and built-in snapshot adapter.                                              |
| `0x1000-0xffff` | Game-defined messages. Use this range for project-specific input, actions, replication, combat, inventory, and other gameplay traffic. |

## Registering A Game Message

Register custom messages on the server or runtime before starting the transport:

=== "GoLang"

    ```go
    type CastSpell struct {
    	PlayerID string
    	SpellID  uint16
    }

    err := server.RegisterWireMessage(network.WireMessageDefinition{
    	ID:          network.WireMessageGameMin + 10,
    	Name:        "game.cast_spell",
    	Direction:   network.WireDirectionClientToServer,
    	MessageType: reflect.TypeOf(CastSpell{}),
    	Encode: func(message any) ([]byte, error) {
    		cast := message.(CastSpell)
    		var buffer bytes.Buffer
    		// Write a bounded string and fixed-width fields in your chosen schema.
    		binary.Write(&buffer, binary.BigEndian, uint16(len(cast.PlayerID)))
    		buffer.WriteString(cast.PlayerID)
    		binary.Write(&buffer, binary.BigEndian, cast.SpellID)
    		return buffer.Bytes(), nil
    	},
    	Decode: func(payload []byte) (any, error) {
    		reader := bytes.NewReader(payload)
    		var length uint16
    		if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
    			return nil, err
    		}
    		playerID := make([]byte, length)
    		if _, err := reader.Read(playerID); err != nil {
    			return nil, err
    		}
    		var spellID uint16
    		if err := binary.Read(reader, binary.BigEndian, &spellID); err != nil {
    			return nil, err
    		}
    		return CastSpell{PlayerID: string(playerID), SpellID: spellID}, nil
    	},
    	Validate: func(message any) error {
    		cast := message.(CastSpell)
    		if cast.PlayerID == "" {
    			return errors.New("missing player id")
    		}
    		return nil
    	},
    	Handler: func(ctx network.WireMessageContext) error {
    		cast := ctx.Message.(CastSpell)
    		// Route into your game systems using ctx.Runtime, ctx.ClientID, or ctx.Response.
    		_ = cast
    		return nil
    	},
    })
    ```

=== "C#"

    ```csharp
    using System.Buffers.Binary;
    using System.Text;

    public readonly record struct CastSpell(string PlayerId, ushort SpellId);

    server.RegisterWireMessage(new WireMessageDefinition
    {
        Id = EnservaWire.GameMin + 10,
        Name = "game.cast_spell",
        Direction = WireMessageDirection.ClientToServer,
        MessageType = typeof(CastSpell),
        Encode = message =>
        {
            var cast = (CastSpell)message;
            byte[] playerId = Encoding.UTF8.GetBytes(cast.PlayerId);
            byte[] payload = new byte[2 + playerId.Length + 2];
            BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(0, 2), (ushort)playerId.Length);
            playerId.CopyTo(payload.AsSpan(2));
            BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(2 + playerId.Length, 2), cast.SpellId);
            return payload;
        },
        Decode = payload =>
        {
            ushort length = BinaryPrimitives.ReadUInt16BigEndian(payload.AsSpan(0, 2));
            string playerId = Encoding.UTF8.GetString(payload.AsSpan(2, length));
            ushort spellId = BinaryPrimitives.ReadUInt16BigEndian(payload.AsSpan(2 + length, 2));
            return new CastSpell(playerId, spellId);
        },
        Validate = message =>
        {
            var cast = (CastSpell)message;
            if (string.IsNullOrWhiteSpace(cast.PlayerId))
                throw new InvalidOperationException("missing player id");
        },
    });
    ```

=== "Rust"

    ```rust
    use enserva_rust_client_example::{DeliveryClass, EnservaUdpClient};

    struct CastSpell {
        player_id: String,
        spell_id: u16,
    }

    fn encode_cast_spell(cast: &CastSpell) -> Vec<u8> {
        let mut payload = Vec::new();
        payload.extend_from_slice(&(cast.player_id.len() as u16).to_be_bytes());
        payload.extend_from_slice(cast.player_id.as_bytes());
        payload.extend_from_slice(&cast.spell_id.to_be_bytes());
        payload
    }

    let payload = encode_cast_spell(&CastSpell {
        player_id: "player-1".into(),
        spell_id: 42,
    });

    client.send_custom_message(0x1000 + 10, &payload, DeliveryClass::Unreliable)?;
    ```

The registry owns message encoding, decoding, validation, and optional dispatch. The packet framing layer only sees message IDs and bytes, so new game messages do not require changes to UDP transport or packet parsing code.

To opt a registered message into reliable delivery when it is sent through the UDP response path, set the delivery field:

```go
Delivery: network.DeliveryReliableOrdered,
```

```rust
delivery: DeliveryClass::ReliableOrdered,
```

For one-off responses, wrap the response value:

```go
return ctx.Respond(network.DeliverReliableUnordered(MyReply{ID: id}))
```

```rust
ctx.respond(Delivery::reliable_unordered(MyReply { id }))?;
```

Do not mark high-rate snapshots reliable unless your client protocol is designed for the extra queueing and retransmission cost.

For compatibility with the existing object model, Enserva registers an engine-level `ObjectRequest` message, a small built-in `PlayerInput` adapter, and a generic tick-aligned `GenericClientInput` envelope. Games can ignore those and register their own messages in the game range.

## Built-In Input Messages

`engine.client_input` (`0x0107`) is a reusable input-buffer envelope:

| Field         | Type     | Meaning                                                                               |
| ------------- | -------- | ------------------------------------------------------------------------------------- |
| `sequence`    | `uint64` | Client input sequence. If zero, the UDP packet sequence is used by the server buffer. |
| `tick`        | `uint64` | Intended simulation/client tick. If zero, the current server tick is used.            |
| `object_type` | string   | Optional target object type.                                                          |
| `object_id`   | string   | Optional target object ID.                                                            |
| `target_id`   | string   | Optional target ID when the input is not tied to one object key.                      |
| `payload`     | bytes    | Opaque game-defined input bytes.                                                      |

The UDP transport buffers `GenericClientInput` by client ID and target tick instead of routing it to object request handlers. Game code consumes buffered inputs from `Runtime.ConsumeClientInputs*` APIs during ticks.

`engine.player_input` (`0x0101`) now carries optional `sequence` and `tick` fields before the existing axes. The decoder still accepts the legacy payload shape of `object_id + x/y/z`; unticked legacy player input is also adapted into the original `player/input` object request path for compatibility. Ticked `PlayerInput` is buffered for runtime consumption.

## Built-In Snapshot Messages

The UDP server sends `engine.world_snapshot` (`0x0102`) for full snapshots. When `Config.EnableDeltaSnapshots` is enabled and a client already has a baseline, it sends `engine.delta_snapshot` (`0x0106`) until `FullSnapshotInterval` forces the next full snapshot.

Delta snapshots are calculated from the current client-visible snapshot after snapshot visibility, scene filtering, and interest management have already run. They contain:

| Field       | Meaning                                                                                           |
| ----------- | ------------------------------------------------------------------------------------------------- |
| `spawned`   | Objects that are new or newly visible to the client.                                              |
| `changed`   | Objects that existed in the previous visible snapshot but whose canonical snapshot value changed. |
| `despawned` | Object type and ID pairs that were previously visible and are now removed or invisible.           |

Full snapshots are also forced when no baseline exists, after authentication changes a client ID, after scene-switch handling, after a client switches from JSON to wire packets, and after timeout/reconnect.

## Tiny UDP Examples

These examples are intentionally bare bones. They only show the packet shape. Production clients should add timeouts, retries, receive loops, sequence tracking, validation, and proper message-specific encoders.

### Wire Player Input

This sends one binary `engine.player_input` message (`0x0101`) inside one wire packet.

=== "GoLang"

    ```go
    conn, err := net.Dial("udp", "127.0.0.1:9000")
    if err != nil {
    	return err
    }
    defer conn.Close()

    input, err := network.EncodeClientMessage(network.PlayerInput{
    	Sequence: 1,
    	Tick:     120,
    	ObjectID: "player-1",
    	X:        1,
    	Y:        0,
    	Z:        0,
    })
    if err != nil {
    	return err
    }

    packet, err := network.EncodePacket(1, []network.WireMessage{input})
    if err != nil {
    	return err
    }

    _, err = conn.Write(packet)
    return err
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

    var messagePayload = new List<byte>();
    WriteString(messagePayload, "player-1"); // objectId
    WriteU64(messagePayload, 1);              // input sequence
    WriteU64(messagePayload, 120);            // target tick
    WriteF32(messagePayload, 1.0f);           // x
    WriteF32(messagePayload, 0.0f);           // y
    WriteF32(messagePayload, 0.0f);           // z

    var messages = new List<byte>();
    WriteU16(messages, 0x0101);                         // engine.player_input
    WriteU32(messages, (uint)messagePayload.Count);
    messages.AddRange(messagePayload);

    var packet = new List<byte>();
    WriteU16(packet, 0x4553);                // magic "ES"
    packet.Add(1);                           // protocol version
    packet.Add(1);                           // message count
    WriteU32(packet, 0);                     // reserved
    WriteU64(packet, 1);                     // sequence
    WriteU64(packet, 0);                     // ack
    WriteU64(packet, 0);                     // ack_bits
    WriteU32(packet, (uint)messages.Count);  // payload length
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

    client.send_player_input("player-1", 1, 120, 1.0, 0.0, 0.0)?;
    ```

### Legacy JSON Request

This sends the same sample input through the supported legacy JSON datagram path.

=== "GoLang"

    ```go
    payload := []byte(`{
      "seq": 1,
      "objectType": "player",
      "objectId": "player-1",
      "action": "input",
      "data": { "x": 1, "y": 0, "z": 0 }
    }`)

    conn, err := net.Dial("udp", "127.0.0.1:9000")
    if err != nil {
    	return err
    }
    defer conn.Close()

    _, err = conn.Write(payload)
    return err
    ```

=== "C#"

    ```csharp
    using System.Net.Sockets;
    using System.Text;

    using var udp = new UdpClient();

    var json = """
    {
      "seq": 1,
      "objectType": "player",
      "objectId": "player-1",
      "action": "input",
      "data": { "x": 1, "y": 0, "z": 0 }
    }
    """;

    byte[] payload = Encoding.UTF8.GetBytes(json);
    await udp.SendAsync(payload, payload.Length, "127.0.0.1", 9000);
    ```

=== "Rust"

    ```rust
    use std::net::UdpSocket;

    let json = r#"{
      "seq": 1,
      "objectType": "player",
      "objectId": "player-1",
      "action": "input",
      "data": { "x": 1, "y": 0, "z": 0 }
    }"#;

    let socket = UdpSocket::bind("0.0.0.0:0")?;
    socket.send_to(json.as_bytes(), "127.0.0.1:9000")?;
    ```
