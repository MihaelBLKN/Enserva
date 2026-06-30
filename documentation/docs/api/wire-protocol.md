# Wire Protocol

Enserva's UDP transport supports a binary packet format for hot-path multiplayer traffic. Legacy JSON request datagrams are still accepted for simple clients and development tooling, but binary clients should use the wire protocol.

## Packet Format

All integer fields are big endian.

| Field | Type | Notes |
| --- | --- | --- |
| `magic` | `uint16` | ASCII `ES`, value `0x4553`. |
| `protocol_version` | `uint8` | Current version is `1`. |
| `message_count` | `uint8` | Number of framed messages in this packet. |
| `reserved` | `uint32` | Reserved for future flags. Send as zero. |
| `sequence` | `uint64` | Packet sequence number from the sender. |
| `ack` | `uint64` | Latest peer packet sequence received by the sender. |
| `ack_bits` | `uint64` | Bit `0` acknowledges `ack - 1`, bit `1` acknowledges `ack - 2`, and so on through 64 prior packets. |
| `payload_length` | `uint32` | Total bytes after the packet header. |

Each message then has:

| Field | Type | Notes |
| --- | --- | --- |
| `message_type` | `uint16` | Stable registered message ID. |
| `payload_length` | `uint32` | Message payload length. |
| `payload` | bytes | Encoded by the registered message definition. |

Oversized packets, malformed lengths, and unsupported protocol versions are rejected before gameplay dispatch.

## Message ID Ranges

| Range | Owner |
| --- | --- |
| `0x0000-0x00ff` | Protocol/system messages such as hello, welcome, ping, pong, error, and disconnect. |
| `0x0100-0x0fff` | Enserva engine messages such as the generic object request and built-in snapshot adapter. |
| `0x1000-0xffff` | Game-defined messages. Use this range for project-specific input, actions, replication, combat, inventory, and other gameplay traffic. |

## Registering A Game Message

Register custom messages on the server or runtime before starting the transport:

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

The registry owns message encoding, decoding, validation, and optional dispatch. The packet framing layer only sees message IDs and bytes, so new game messages do not require changes to UDP transport or packet parsing code.

For compatibility with the existing object model, Enserva registers an engine-level `ObjectRequest` message and a small built-in `PlayerInput` adapter. Games can ignore those and register their own messages in the game range.

## Tiny C# UDP Examples

These examples are intentionally bare bones. They only show the packet shape. Production clients should add timeouts, retries, receive loops, sequence tracking, validation, and proper message-specific encoders.

### Legacy JSON Request

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

### Wire Player Input

This sends one binary `engine.player_input` message (`0x0101`) inside one Wire packet.

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
WriteF32(messagePayload, 1.0f);          // x
WriteF32(messagePayload, 0.0f);          // y
WriteF32(messagePayload, 0.0f);          // z

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
