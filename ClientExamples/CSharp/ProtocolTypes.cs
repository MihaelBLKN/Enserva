#nullable disable

using System;
using System.Collections.Generic;

namespace Enserva.ClientExamples
{
    public enum WireMessageType : ushort
    {
        Unknown = 0x0000,
        ClientHello = 0x0001,
        Welcome = 0x0002,
        Ping = 0x0003,
        Pong = 0x0004,
        Error = 0x0005,
        Disconnect = 0x0006,
        Reliable = 0x0007,

        ObjectRequest = 0x0100,
        PlayerInput = 0x0101,
        WorldSnapshot = 0x0102,
        EntitySpawn = 0x0103,
        EntityDespawn = 0x0104,
        EntityUpdate = 0x0105,
        DeltaSnapshot = 0x0106,
        ClientInput = 0x0107,

        GameMin = 0x1000
    }

    public enum DeliveryClass : byte
    {
        Unreliable = 0,
        ReliableOrdered = 1,
        ReliableUnordered = 2
    }

    [Flags]
    public enum WireCapabilities : ulong
    {
        DeltaSnapshots = 1UL << 0,
        ReliableOrdered = 1UL << 1,
        ReliableUnordered = 1UL << 2
    }

    public sealed class WireMessage
    {
        public WireMessageType Type { get; }
        public byte[] Payload { get; }
        public DeliveryClass Delivery { get; }
        public ulong ReliableId { get; }

        public WireMessage(
            WireMessageType type,
            byte[] payload,
            DeliveryClass delivery = DeliveryClass.Unreliable,
            ulong reliableId = 0)
        {
            Type = type;
            Payload = payload ?? Array.Empty<byte>();
            Delivery = delivery;
            ReliableId = reliableId;
        }
    }

    public sealed class WirePacket
    {
        public ulong Sequence { get; }
        public ulong Ack { get; }
        public ulong AckBits { get; }
        public IReadOnlyList<WireMessage> Messages { get; }

        public WirePacket(ulong sequence, ulong ack, ulong ackBits, IReadOnlyList<WireMessage> messages)
        {
            Sequence = sequence;
            Ack = ack;
            AckBits = ackBits;
            Messages = messages;
        }
    }

    public sealed class WelcomeMessage
    {
        public string ClientId { get; set; }
        public string AuthenticatedId { get; set; }
        public byte ProtocolVersion { get; set; }
        public WireCapabilities Capabilities { get; set; }
        public uint MaxPacketSize { get; set; }
    }

    public sealed class PongMessage
    {
        public ulong Nonce { get; }

        public PongMessage(ulong nonce)
        {
            Nonce = nonce;
        }
    }

    public sealed class ErrorMessage
    {
        public ushort Code { get; set; }
        public string Message { get; set; }
    }

    public sealed class DisconnectMessage
    {
        public ushort Code { get; set; }
        public string Message { get; set; }
    }

    public sealed class WorldSnapshotMessage
    {
        public string ClientId { get; set; }
        public ulong Tick { get; set; }
        public ulong LastSequence { get; set; }
        public Dictionary<string, Dictionary<string, object>> Objects { get; set; }
    }

    public sealed class WorldDeltaSnapshotMessage
    {
        public string ClientId { get; set; }
        public ulong Tick { get; set; }
        public ulong LastSequence { get; set; }
        public ulong BaselineTick { get; set; }
        public Dictionary<string, Dictionary<string, object>> Spawned { get; set; }
        public Dictionary<string, Dictionary<string, object>> Changed { get; set; }
        public List<SnapshotObjectRef> Despawned { get; set; }
    }

    public sealed class SnapshotObjectRef
    {
        public string ObjectType { get; set; }
        public string ObjectId { get; set; }
    }

}
