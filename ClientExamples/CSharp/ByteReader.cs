#nullable disable

using System.Buffers.Binary;
using System.Text;

namespace Enserva.ClientExamples
{
    internal sealed class ByteReader
    {
        private const byte WireValueNull = 0;
        private const byte WireValueBool = 1;
        private const byte WireValueInt64 = 2;
        private const byte WireValueUint64 = 3;
        private const byte WireValueFloat64 = 4;
        private const byte WireValueString = 5;
        private const byte WireValueObject = 6;
        private const byte WireValueList = 7;

        private readonly byte[] bytes;
        private int position;

        public ByteReader(byte[] bytes)
        {
            this.bytes = bytes ?? Array.Empty<byte>();
        }

        public int Remaining => bytes.Length - position;

        public byte ReadByte()
        {
            Ensure(1);
            return bytes[position++];
        }

        public ushort ReadUInt16()
        {
            Ensure(2);
            ushort value = BinaryPrimitives.ReadUInt16BigEndian(bytes.AsSpan(position, 2));
            position += 2;
            return value;
        }

        public uint ReadUInt32()
        {
            Ensure(4);
            uint value = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(position, 4));
            position += 4;
            return value;
        }

        public ulong ReadUInt64()
        {
            Ensure(8);
            ulong value = BinaryPrimitives.ReadUInt64BigEndian(bytes.AsSpan(position, 8));
            position += 8;
            return value;
        }

        public double ReadFloat64()
        {
            return BitConverter.Int64BitsToDouble((long)ReadUInt64());
        }

        public string ReadString()
        {
            ushort length = ReadUInt16();
            Ensure(length);
            string value = Encoding.UTF8.GetString(bytes, position, length);
            position += length;
            return value;
        }

        public byte[] ReadBytes(int length)
        {
            Ensure(length);
            byte[] value = new byte[length];
            Buffer.BlockCopy(bytes, position, value, 0, length);
            position += length;
            return value;
        }

        public byte[] ReadLengthPrefixedBytes(int maxLength)
        {
            uint length = ReadUInt32();
            if (length > maxLength)
                throw new InvalidOperationException("Length-prefixed payload is too large: " + length);

            return ReadBytes((int)length);
        }

        public Dictionary<string, Dictionary<string, object>> ReadSnapshotData()
        {
            ushort typeCount = ReadUInt16();
            Dictionary<string, Dictionary<string, object>> snapshot =
                new Dictionary<string, Dictionary<string, object>>(typeCount);

            for (int typeIndex = 0; typeIndex < typeCount; typeIndex++)
            {
                string objectType = ReadString();
                ushort objectCount = ReadUInt16();
                Dictionary<string, object> objectsById = new Dictionary<string, object>(objectCount);

                for (int objectIndex = 0; objectIndex < objectCount; objectIndex++)
                {
                    string objectId = ReadString();
                    objectsById[objectId] = ReadWireValue(0);
                }

                snapshot[objectType] = objectsById;
            }

            return snapshot;
        }

        public List<SnapshotObjectRef> ReadSnapshotObjectRefs()
        {
            ushort count = ReadUInt16();
            List<SnapshotObjectRef> refs = new List<SnapshotObjectRef>(count);

            for (int index = 0; index < count; index++)
            {
                refs.Add(new SnapshotObjectRef
                {
                    ObjectType = ReadString(),
                    ObjectId = ReadString()
                });
            }

            return refs;
        }

        public object ReadWireValue(int depth)
        {
            if (depth > 16)
                throw new InvalidOperationException("Wire value nesting is too deep.");

            byte kind = ReadByte();
            switch (kind)
            {
                case WireValueNull:
                    return null;
                case WireValueBool:
                    return ReadByte() != 0;
                case WireValueInt64:
                    return unchecked((long)ReadUInt64());
                case WireValueUint64:
                    return ReadUInt64();
                case WireValueFloat64:
                    return ReadFloat64();
                case WireValueString:
                    return ReadString();
                case WireValueObject:
                    {
                        ushort count = ReadUInt16();
                        Dictionary<string, object> value = new Dictionary<string, object>(count);
                        for (int index = 0; index < count; index++)
                            value[ReadString()] = ReadWireValue(depth + 1);
                        return value;
                    }
                case WireValueList:
                    {
                        ushort count = ReadUInt16();
                        List<object> value = new List<object>(count);
                        for (int index = 0; index < count; index++)
                            value.Add(ReadWireValue(depth + 1));
                        return value;
                    }
                default:
                    throw new InvalidOperationException("Unknown wire value kind: " + kind);
            }
        }

        private void Ensure(int length)
        {
            if (length < 0 || Remaining < length)
                throw new InvalidOperationException("Wire payload is truncated.");
        }
    }
}
