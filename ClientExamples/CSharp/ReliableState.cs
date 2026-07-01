#nullable disable

using System;
using System.Collections.Generic;

namespace Enserva.ClientExamples
{
    internal sealed class PendingReliableMessage
    {
        public WireMessage Message;
        public HashSet<ulong> SentPacketSequences = new HashSet<ulong>();
        public int Attempts;
        public int MaxAttempts;
        public TimeSpan RetryInterval;
        public DateTime NextRetryAtUtc;
    }

}
