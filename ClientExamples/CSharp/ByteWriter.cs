#nullable disable

using System.Buffers.Binary;
using System.Text;

namespace Enserva.ClientExamples
{
    internal sealed class ByteWriter
    {
        private const int MaxWireStringBytes = 2048;

        private readonly List<byte> bytes = new List<byte>();

        public void WriteByte(byte value)
        {
            bytes.Add(value);
        }

        public void WriteUInt16(ushort value)
        {
            byte[] buffer = new byte[2];
            BinaryPrimitives.WriteUInt16BigEndian(buffer, value);
            bytes.AddRange(buffer);
        }

        public void WriteUInt32(uint value)
        {
            byte[] buffer = new byte[4];
            BinaryPrimitives.WriteUInt32BigEndian(buffer, value);
            bytes.AddRange(buffer);
        }

        public void WriteUInt64(ulong value)
        {
            byte[] buffer = new byte[8];
            BinaryPrimitives.WriteUInt64BigEndian(buffer, value);
            bytes.AddRange(buffer);
        }

        public void WriteFloat32(float value)
        {
            WriteUInt32((uint)BitConverter.SingleToInt32Bits(value));
        }

        public void WriteString(string value)
        {
            byte[] text = Encoding.UTF8.GetBytes(value ?? string.Empty);
            if (text.Length > MaxWireStringBytes || text.Length > ushort.MaxValue)
                throw new InvalidOperationException("Wire string is too large: " + text.Length);

            WriteUInt16((ushort)text.Length);
            bytes.AddRange(text);
        }

        public void WriteBytes(byte[] value)
        {
            byte[] data = value ?? Array.Empty<byte>();
            WriteUInt32((uint)data.Length);
            bytes.AddRange(data);
        }

        public void WriteBytesRaw(byte[] value)
        {
            bytes.AddRange(value ?? Array.Empty<byte>());
        }

        public byte[] ToArray()
        {
            return bytes.ToArray();
        }
    }

}
