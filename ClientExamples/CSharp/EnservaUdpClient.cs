#nullable disable

using System;
using System.Collections.Generic;
using System.Net.Sockets;
using System.Text;
using System.Threading;
using System.Threading.Tasks;

namespace Enserva.ClientExamples
{
    // Drop this file into a C# game client or engine-side networking layer to talk
    // to Enserva's UDP wire protocol. It covers the built-in packet framing,
    // authentication hello, object requests, player input, generic client input,
    // ping/pong, snapshots, custom game-message payloads, and basic reliable retry.
    public sealed class EnservaUdpClient : IDisposable
    {
        private const ushort Magic = 0x4553; // ASCII "ES"
        private const byte ProtocolVersion = 1;
        private const int PacketHeaderSize = 36;
        private const int MessageHeaderSize = 6;
        private const int MaxMessagesPerPacket = 32;
        private const int MaxWireStringBytes = 2048;
        private const int MaxWireMessagePayloadSize = 48 * 1024;

        private readonly object gate = new object();
        private readonly List<PendingReliableMessage> pendingReliable = new List<PendingReliableMessage>();
        private readonly HashSet<ulong> receivedSequences = new HashSet<ulong>();
        private readonly HashSet<ulong> reliableUnorderedReceived = new HashSet<ulong>();
        private readonly HashSet<ulong> reliableOrderedReceived = new HashSet<ulong>();
        private readonly SortedDictionary<ulong, WireMessage> reliableOrderedBuffer = new SortedDictionary<ulong, WireMessage>();

        private UdpClient udp;
        private ulong nextPacketSequence = 1;
        private ulong nextReliableOrderedId = 1;
        private ulong nextReliableUnorderedId = 1;
        private ulong latestPeerSequence;
        private ulong reliableOrderedNext = 1;
        private bool disposed;

        public event Action<WelcomeMessage> WelcomeReceived;
        public event Action<PongMessage> PongReceived;
        public event Action<ErrorMessage> ErrorReceived;
        public event Action<DisconnectMessage> DisconnectReceived;
        public event Action<WorldSnapshotMessage> WorldSnapshotReceived;
        public event Action<WorldDeltaSnapshotMessage> WorldDeltaSnapshotReceived;
        public event Action<WireMessage> CustomMessageReceived;
        public event Action<Exception> ReceiveError;

        public string ClientId { get; private set; }
        public string AuthenticatedId { get; private set; }
        public WireCapabilities NegotiatedCapabilities { get; private set; }
        public uint NegotiatedMaxPacketSize { get; private set; }

        public async Task ConnectAsync(
            string host,
            int port,
            string clientName,
            string token,
            uint maxPacketSize = 1200,
            WireCapabilities capabilities =
                WireCapabilities.DeltaSnapshots |
                WireCapabilities.ReliableOrdered |
                WireCapabilities.ReliableUnordered)
        {
            ThrowIfDisposed();

            udp = new UdpClient();
            udp.Connect(host, port);

            await SendClientHelloAsync(clientName, token, capabilities, maxPacketSize).ConfigureAwait(false);
        }

        // Most engines call this from an async task started when the game connects.
        // Stop it by cancelling the token or disposing the client.
        public async Task RunReceiveLoopAsync(CancellationToken cancellationToken)
        {
            ThrowIfDisposed();
            if (udp == null)
                throw new InvalidOperationException("Call ConnectAsync before starting the receive loop.");

            using (cancellationToken.Register(() => udp.Close()))
            {
                while (!cancellationToken.IsCancellationRequested)
                {
                    try
                    {
                        UdpReceiveResult result = await udp.ReceiveAsync().ConfigureAwait(false);
                        HandleDatagram(result.Buffer);
                    }
                    catch (ObjectDisposedException) when (cancellationToken.IsCancellationRequested)
                    {
                        break;
                    }
                    catch (SocketException) when (cancellationToken.IsCancellationRequested)
                    {
                        break;
                    }
                    catch (Exception ex)
                    {
                        ReceiveError?.Invoke(ex);
                    }
                }
            }
        }

        // Call this from the engine update loop, for example once per frame, so
        // client-to-server reliable messages are resent until the server acks a
        // packet sequence that carried them.
        public async Task UpdateAsync()
        {
            ThrowIfDisposed();

            List<PendingReliableMessage> due = new List<PendingReliableMessage>();
            DateTime now = DateTime.UtcNow;

            lock (gate)
            {
                for (int index = pendingReliable.Count - 1; index >= 0; index--)
                {
                    PendingReliableMessage pending = pendingReliable[index];
                    if (pending.Attempts >= pending.MaxAttempts)
                    {
                        pendingReliable.RemoveAt(index);
                        continue;
                    }

                    if (pending.NextRetryAtUtc <= now)
                        due.Add(pending);
                }
            }

            foreach (PendingReliableMessage pending in due)
            {
                ulong sequence = await SendPacketAsync(new[] { pending.Message }).ConfigureAwait(false);
                lock (gate)
                {
                    pending.Attempts++;
                    pending.SentPacketSequences.Add(sequence);
                    pending.NextRetryAtUtc = DateTime.UtcNow + pending.RetryInterval;
                }
            }
        }

        public Task SendClientHelloAsync(
            string clientName,
            string token,
            WireCapabilities capabilities,
            uint maxPacketSize)
        {
            ByteWriter writer = new ByteWriter();
            writer.WriteString(clientName);
            writer.WriteString(token);
            writer.WriteByte(ProtocolVersion);
            writer.WriteUInt64((ulong)capabilities);
            writer.WriteUInt32(maxPacketSize);

            return SendMessageAsync(new WireMessage(WireMessageType.ClientHello, writer.ToArray()));
        }

        public Task SendPingAsync(ulong nonce)
        {
            ByteWriter writer = new ByteWriter();
            writer.WriteUInt64(nonce);
            return SendMessageAsync(new WireMessage(WireMessageType.Ping, writer.ToArray()));
        }

        // Useful after receiving reliable server messages when the client has no
        // gameplay input to send. Enserva packets need at least one message, so a
        // zero-nonce ping is used as a tiny ack carrier.
        public Task SendKeepAliveAsync()
        {
            return SendPingAsync(0);
        }

        public Task SendPlayerInputAsync(
            string objectId,
            ulong inputSequence,
            ulong tick,
            float x,
            float y,
            float z)
        {
            ByteWriter writer = new ByteWriter();
            writer.WriteString(objectId);
            writer.WriteUInt64(inputSequence);
            writer.WriteUInt64(tick);
            writer.WriteFloat32(x);
            writer.WriteFloat32(y);
            writer.WriteFloat32(z);

            return SendMessageAsync(new WireMessage(WireMessageType.PlayerInput, writer.ToArray()));
        }

        // Sends Enserva's engine.client_input envelope. The payload is your own
        // game-defined bytes, buffered by the server for the target tick.
        public Task SendClientInputAsync(
            ulong inputSequence,
            ulong tick,
            string objectType,
            string objectId,
            string targetId,
            byte[] payload)
        {
            ByteWriter writer = new ByteWriter();
            writer.WriteUInt64(inputSequence);
            writer.WriteUInt64(tick);
            writer.WriteString(objectType);
            writer.WriteString(objectId);
            writer.WriteString(targetId);
            writer.WriteBytes(payload ?? Array.Empty<byte>());

            return SendMessageAsync(new WireMessage(WireMessageType.ClientInput, writer.ToArray()));
        }

        // Compatibility path for existing object handlers. Data is usually UTF-8
        // JSON, for example: {"x":1,"y":0,"z":0}
        public Task SendObjectRequestAsync(
            string objectType,
            string objectId,
            string action,
            string jsonData,
            DeliveryClass delivery = DeliveryClass.Unreliable)
        {
            byte[] data = string.IsNullOrEmpty(jsonData)
                ? Array.Empty<byte>()
                : Encoding.UTF8.GetBytes(jsonData);

            return SendObjectRequestAsync(objectType, objectId, action, data, delivery);
        }

        public Task SendObjectRequestAsync(
            string objectType,
            string objectId,
            string action,
            byte[] data,
            DeliveryClass delivery = DeliveryClass.Unreliable)
        {
            ByteWriter writer = new ByteWriter();
            writer.WriteString(objectType);
            writer.WriteString(objectId);
            writer.WriteString(action);
            writer.WriteBytes(data ?? Array.Empty<byte>());

            return SendMessageAsync(CreateOutgoingMessage(WireMessageType.ObjectRequest, writer.ToArray(), delivery));
        }

        // Custom game messages should use IDs from 0x1000 through 0xffff. Encode
        // the payload here using the same schema registered on the Go server.
        public Task SendCustomMessageAsync(
            ushort messageType,
            byte[] payload,
            DeliveryClass delivery = DeliveryClass.Unreliable)
        {
            return SendMessageAsync(CreateOutgoingMessage((WireMessageType)messageType, payload ?? Array.Empty<byte>(), delivery));
        }

        // Tiny legacy escape hatch for scripts/tools. New gameplay clients should
        // prefer wire messages above.
        public async Task SendLegacyJsonDatagramAsync(string json)
        {
            ThrowIfDisposed();
            if (udp == null)
                throw new InvalidOperationException("Call ConnectAsync before sending.");

            byte[] payload = Encoding.UTF8.GetBytes(json);
            await udp.SendAsync(payload, payload.Length).ConfigureAwait(false);
        }

        private async Task SendMessageAsync(WireMessage message)
        {
            bool reliable = message.Delivery == DeliveryClass.ReliableOrdered ||
                            message.Delivery == DeliveryClass.ReliableUnordered;

            ulong sequence = await SendPacketAsync(new[] { message }).ConfigureAwait(false);

            if (reliable)
                TrackReliableMessage(message, sequence);
        }

        private async Task<ulong> SendPacketAsync(IReadOnlyList<WireMessage> messages)
        {
            ThrowIfDisposed();
            if (udp == null)
                throw new InvalidOperationException("Call ConnectAsync before sending.");

            ulong sequence;
            ulong ack;
            ulong ackBits;

            lock (gate)
            {
                sequence = nextPacketSequence++;
                ack = latestPeerSequence;
                ackBits = BuildAckBitsLocked();
            }

            byte[] packet = EncodePacket(sequence, ack, ackBits, messages);
            await udp.SendAsync(packet, packet.Length).ConfigureAwait(false);
            return sequence;
        }

        private WireMessage CreateOutgoingMessage(WireMessageType type, byte[] payload, DeliveryClass delivery)
        {
            if (delivery == DeliveryClass.Unreliable)
                return new WireMessage(type, payload);

            ulong reliableId;
            lock (gate)
            {
                if (delivery == DeliveryClass.ReliableOrdered)
                    reliableId = nextReliableOrderedId++;
                else if (delivery == DeliveryClass.ReliableUnordered)
                    reliableId = nextReliableUnorderedId++;
                else
                    throw new InvalidOperationException("Unknown delivery class: " + delivery);
            }

            return new WireMessage(type, payload, delivery, reliableId);
        }

        private byte[] EncodePacket(ulong sequence, ulong ack, ulong ackBits, IReadOnlyList<WireMessage> messages)
        {
            if (messages == null || messages.Count == 0 || messages.Count > MaxMessagesPerPacket)
                throw new InvalidOperationException("Enserva packets must contain 1..32 messages.");

            ByteWriter payload = new ByteWriter();
            foreach (WireMessage message in messages)
            {
                WireMessageType wireType = message.Type;
                byte[] wirePayload = message.Payload ?? Array.Empty<byte>();

                if (message.Delivery == DeliveryClass.ReliableOrdered ||
                    message.Delivery == DeliveryClass.ReliableUnordered)
                {
                    wireType = WireMessageType.Reliable;
                    wirePayload = EncodeReliableEnvelope(message);
                }

                if (wirePayload.Length > MaxWireMessagePayloadSize)
                    throw new InvalidOperationException("Wire message payload is too large: " + wirePayload.Length);

                payload.WriteUInt16((ushort)wireType);
                payload.WriteUInt32((uint)wirePayload.Length);
                payload.WriteBytesRaw(wirePayload);
            }

            byte[] framedMessages = payload.ToArray();

            ByteWriter packet = new ByteWriter();
            packet.WriteUInt16(Magic);
            packet.WriteByte(ProtocolVersion);
            packet.WriteByte((byte)messages.Count);
            packet.WriteUInt32(0);
            packet.WriteUInt64(sequence);
            packet.WriteUInt64(ack);
            packet.WriteUInt64(ackBits);
            packet.WriteUInt32((uint)framedMessages.Length);
            packet.WriteBytesRaw(framedMessages);
            return packet.ToArray();
        }

        private byte[] EncodeReliableEnvelope(WireMessage message)
        {
            if (message.ReliableId == 0)
                throw new InvalidOperationException("Reliable messages require a non-zero reliable id.");

            ByteWriter writer = new ByteWriter();
            writer.WriteByte((byte)message.Delivery);
            writer.WriteUInt64(message.ReliableId);
            writer.WriteUInt16((ushort)message.Type);
            writer.WriteUInt32((uint)(message.Payload == null ? 0 : message.Payload.Length));
            writer.WriteBytesRaw(message.Payload ?? Array.Empty<byte>());
            return writer.ToArray();
        }

        private void HandleDatagram(byte[] datagram)
        {
            if (datagram == null || datagram.Length == 0)
                return;

            if (!LooksLikeWirePacket(datagram))
                return;

            WirePacket packet = DecodePacket(datagram);
            RecordPeerPacket(packet.Sequence);
            RemoveAckedReliableMessages(packet.Ack, packet.AckBits);

            foreach (WireMessage message in AcceptIncomingReliableMessages(packet.Messages))
                DispatchServerMessage(message);
        }

        private WirePacket DecodePacket(byte[] data)
        {
            if (data.Length < PacketHeaderSize)
                throw new InvalidOperationException("Wire packet header is too short.");

            ByteReader reader = new ByteReader(data);
            ushort magic = reader.ReadUInt16();
            if (magic != Magic)
                throw new InvalidOperationException("Bad Enserva wire magic.");

            byte version = reader.ReadByte();
            if (version != ProtocolVersion)
                throw new InvalidOperationException("Unsupported Enserva wire protocol version: " + version);

            int messageCount = reader.ReadByte();
            if (messageCount <= 0 || messageCount > MaxMessagesPerPacket)
                throw new InvalidOperationException("Invalid Enserva message count: " + messageCount);

            reader.ReadUInt32(); // reserved
            ulong sequence = reader.ReadUInt64();
            ulong ack = reader.ReadUInt64();
            ulong ackBits = reader.ReadUInt64();
            uint payloadLength = reader.ReadUInt32();
            if (payloadLength != data.Length - PacketHeaderSize)
                throw new InvalidOperationException("Wire packet payload length mismatch.");

            List<WireMessage> messages = new List<WireMessage>(messageCount);
            for (int index = 0; index < messageCount; index++)
            {
                if (reader.Remaining < MessageHeaderSize)
                    throw new InvalidOperationException("Wire message header is truncated.");

                WireMessageType type = (WireMessageType)reader.ReadUInt16();
                uint length = reader.ReadUInt32();
                if (length > MaxWireMessagePayloadSize)
                    throw new InvalidOperationException("Wire message payload is too large: " + length);
                if (length > reader.Remaining)
                    throw new InvalidOperationException("Wire message payload is truncated.");

                byte[] payload = reader.ReadBytes((int)length);
                messages.Add(type == WireMessageType.Reliable
                    ? DecodeReliableEnvelope(payload)
                    : new WireMessage(type, payload));
            }

            if (reader.Remaining != 0)
                throw new InvalidOperationException("Wire packet has trailing bytes.");

            return new WirePacket(sequence, ack, ackBits, messages);
        }

        private WireMessage DecodeReliableEnvelope(byte[] payload)
        {
            ByteReader reader = new ByteReader(payload);
            DeliveryClass delivery = (DeliveryClass)reader.ReadByte();
            if (delivery != DeliveryClass.ReliableOrdered &&
                delivery != DeliveryClass.ReliableUnordered)
                throw new InvalidOperationException("Invalid reliable delivery class: " + delivery);

            ulong reliableId = reader.ReadUInt64();
            WireMessageType innerType = (WireMessageType)reader.ReadUInt16();
            uint innerLength = reader.ReadUInt32();
            if (innerLength > reader.Remaining)
                throw new InvalidOperationException("Reliable payload is truncated.");

            byte[] innerPayload = reader.ReadBytes((int)innerLength);
            if (reader.Remaining != 0)
                throw new InvalidOperationException("Reliable envelope has trailing bytes.");

            return new WireMessage(innerType, innerPayload, delivery, reliableId);
        }

        private void DispatchServerMessage(WireMessage message)
        {
            ByteReader reader = new ByteReader(message.Payload ?? Array.Empty<byte>());

            switch (message.Type)
            {
                case WireMessageType.Welcome:
                {
                    WelcomeMessage welcome = DecodeWelcome(reader);
                    ClientId = welcome.ClientId;
                    AuthenticatedId = welcome.AuthenticatedId;
                    NegotiatedCapabilities = welcome.Capabilities;
                    NegotiatedMaxPacketSize = welcome.MaxPacketSize;
                    WelcomeReceived?.Invoke(welcome);
                    break;
                }
                case WireMessageType.Pong:
                    PongReceived?.Invoke(new PongMessage(reader.ReadUInt64()));
                    break;
                case WireMessageType.Error:
                    ErrorReceived?.Invoke(DecodeError(reader));
                    break;
                case WireMessageType.Disconnect:
                    DisconnectReceived?.Invoke(DecodeDisconnect(reader));
                    break;
                case WireMessageType.WorldSnapshot:
                    WorldSnapshotReceived?.Invoke(DecodeWorldSnapshot(reader));
                    break;
                case WireMessageType.DeltaSnapshot:
                    WorldDeltaSnapshotReceived?.Invoke(DecodeWorldDeltaSnapshot(reader));
                    break;
                default:
                    CustomMessageReceived?.Invoke(message);
                    break;
            }
        }

        private WelcomeMessage DecodeWelcome(ByteReader reader)
        {
            string clientId = reader.ReadString();
            string authenticatedId = reader.ReadString();
            byte version = 0;
            WireCapabilities capabilities = 0;
            uint maxPacketSize = 0;

            if (reader.Remaining > 0)
            {
                if (reader.Remaining < 13)
                    throw new InvalidOperationException("Welcome negotiation data is truncated.");

                version = reader.ReadByte();
                capabilities = (WireCapabilities)reader.ReadUInt64();
                maxPacketSize = reader.ReadUInt32();
            }

            return new WelcomeMessage
            {
                ClientId = clientId,
                AuthenticatedId = authenticatedId,
                ProtocolVersion = version,
                Capabilities = capabilities,
                MaxPacketSize = maxPacketSize
            };
        }

        private ErrorMessage DecodeError(ByteReader reader)
        {
            return new ErrorMessage
            {
                Code = reader.ReadUInt16(),
                Message = reader.ReadString()
            };
        }

        private DisconnectMessage DecodeDisconnect(ByteReader reader)
        {
            return new DisconnectMessage
            {
                Code = reader.ReadUInt16(),
                Message = reader.ReadString()
            };
        }

        private WorldSnapshotMessage DecodeWorldSnapshot(ByteReader reader)
        {
            return new WorldSnapshotMessage
            {
                ClientId = reader.ReadString(),
                Tick = reader.ReadUInt64(),
                LastSequence = reader.ReadUInt64(),
                Objects = reader.ReadSnapshotData()
            };
        }

        private WorldDeltaSnapshotMessage DecodeWorldDeltaSnapshot(ByteReader reader)
        {
            return new WorldDeltaSnapshotMessage
            {
                ClientId = reader.ReadString(),
                Tick = reader.ReadUInt64(),
                LastSequence = reader.ReadUInt64(),
                BaselineTick = reader.ReadUInt64(),
                Spawned = reader.ReadSnapshotData(),
                Changed = reader.ReadSnapshotData(),
                Despawned = reader.ReadSnapshotObjectRefs()
            };
        }

        private List<WireMessage> AcceptIncomingReliableMessages(IReadOnlyList<WireMessage> messages)
        {
            List<WireMessage> accepted = new List<WireMessage>();

            lock (gate)
            {
                foreach (WireMessage message in messages)
                {
                    if (message.Delivery == DeliveryClass.Unreliable)
                    {
                        accepted.Add(message);
                        continue;
                    }

                    if (message.ReliableId == 0)
                        continue;

                    if (message.Delivery == DeliveryClass.ReliableUnordered)
                    {
                        if (reliableUnorderedReceived.Add(message.ReliableId))
                            accepted.Add(message);
                        continue;
                    }

                    if (message.Delivery == DeliveryClass.ReliableOrdered)
                    {
                        if (message.ReliableId < reliableOrderedNext ||
                            reliableOrderedReceived.Contains(message.ReliableId) ||
                            reliableOrderedBuffer.ContainsKey(message.ReliableId))
                        {
                            continue;
                        }

                        if (message.ReliableId > reliableOrderedNext)
                        {
                            reliableOrderedBuffer[message.ReliableId] = message;
                            continue;
                        }

                        AcceptOrderedMessageLocked(message, accepted);
                    }
                }
            }

            return accepted;
        }

        private void AcceptOrderedMessageLocked(WireMessage message, List<WireMessage> accepted)
        {
            accepted.Add(message);
            reliableOrderedReceived.Add(message.ReliableId);
            reliableOrderedNext++;

            while (reliableOrderedBuffer.TryGetValue(reliableOrderedNext, out WireMessage buffered))
            {
                reliableOrderedBuffer.Remove(reliableOrderedNext);
                accepted.Add(buffered);
                reliableOrderedReceived.Add(buffered.ReliableId);
                reliableOrderedNext++;
            }
        }

        private void TrackReliableMessage(WireMessage message, ulong packetSequence)
        {
            PendingReliableMessage pending = new PendingReliableMessage
            {
                Message = message,
                RetryInterval = TimeSpan.FromMilliseconds(100),
                NextRetryAtUtc = DateTime.UtcNow + TimeSpan.FromMilliseconds(100),
                MaxAttempts = 5,
                Attempts = 1
            };
            pending.SentPacketSequences.Add(packetSequence);

            lock (gate)
            {
                pendingReliable.Add(pending);
            }
        }

        private void RemoveAckedReliableMessages(ulong ack, ulong ackBits)
        {
            lock (gate)
            {
                for (int index = pendingReliable.Count - 1; index >= 0; index--)
                {
                    PendingReliableMessage pending = pendingReliable[index];
                    foreach (ulong sequence in pending.SentPacketSequences)
                    {
                        if (PacketSequenceAcked(ack, ackBits, sequence))
                        {
                            pendingReliable.RemoveAt(index);
                            break;
                        }
                    }
                }
            }
        }

        private void RecordPeerPacket(ulong sequence)
        {
            if (sequence == 0)
                return;

            lock (gate)
            {
                receivedSequences.Add(sequence);
                if (sequence > latestPeerSequence)
                    latestPeerSequence = sequence;

                if (latestPeerSequence > 64)
                {
                    ulong minimum = latestPeerSequence - 64;
                    receivedSequences.RemoveWhere(item => item < minimum);
                }
            }
        }

        private ulong BuildAckBitsLocked()
        {
            if (latestPeerSequence == 0)
                return 0;

            ulong bits = 0;
            for (ulong offset = 0; offset < 64; offset++)
            {
                ulong sequence = latestPeerSequence - offset - 1;
                if (receivedSequences.Contains(sequence))
                    bits |= 1UL << (int)offset;
                if (sequence == 0)
                    break;
            }

            return bits;
        }

        private static bool PacketSequenceAcked(ulong ack, ulong ackBits, ulong sequence)
        {
            if (sequence == 0 || ack == 0)
                return false;
            if (sequence == ack)
                return true;
            if (sequence > ack)
                return false;

            ulong distance = ack - sequence;
            if (distance == 0 || distance > 64)
                return false;

            return (ackBits & (1UL << (int)(distance - 1))) != 0;
        }

        private static bool LooksLikeWirePacket(byte[] payload)
        {
            return payload.Length >= 2 &&
                   payload[0] == (byte)(Magic >> 8) &&
                   payload[1] == (byte)(Magic & 0xff);
        }

        private void ThrowIfDisposed()
        {
            if (disposed)
                throw new ObjectDisposedException(nameof(EnservaUdpClient));
        }

        public void Dispose()
        {
            disposed = true;
            udp?.Close();
            udp?.Dispose();
        }
    }
}
