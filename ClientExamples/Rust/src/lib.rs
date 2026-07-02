use std::collections::{BTreeMap, BTreeSet, HashSet, VecDeque};
use std::io;
use std::net::{ToSocketAddrs, UdpSocket};
use std::string::FromUtf8Error;
use std::time::{Duration, Instant};

const MAGIC: u16 = 0x4553;
const PROTOCOL_VERSION: u8 = 1;
const PACKET_HEADER_SIZE: usize = 36;
const MESSAGE_HEADER_SIZE: usize = 6;
const MAX_MESSAGES_PER_PACKET: usize = 32;
const MAX_WIRE_STRING_BYTES: usize = 2048;
const MAX_WIRE_MESSAGE_PAYLOAD_SIZE: usize = 48 * 1024;

#[derive(Debug)]
pub enum EnservaError {
    Io(io::Error),
    Utf8(FromUtf8Error),
    Protocol(String),
}

impl std::fmt::Display for EnservaError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Io(error) => write!(formatter, "I/O error: {error}"),
            Self::Utf8(error) => write!(formatter, "UTF-8 error: {error}"),
            Self::Protocol(message) => write!(formatter, "protocol error: {message}"),
        }
    }
}

impl std::error::Error for EnservaError {}

impl From<io::Error> for EnservaError {
    fn from(error: io::Error) -> Self {
        Self::Io(error)
    }
}

impl From<FromUtf8Error> for EnservaError {
    fn from(error: FromUtf8Error) -> Self {
        Self::Utf8(error)
    }
}

pub type Result<T> = std::result::Result<T, EnservaError>;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct WireMessageType(u16);

impl WireMessageType {
    pub const CLIENT_HELLO: Self = Self(0x0001);
    pub const WELCOME: Self = Self(0x0002);
    pub const PING: Self = Self(0x0003);
    pub const PONG: Self = Self(0x0004);
    pub const ERROR: Self = Self(0x0005);
    pub const DISCONNECT: Self = Self(0x0006);
    pub const RELIABLE: Self = Self(0x0007);
    pub const OBJECT_REQUEST: Self = Self(0x0100);
    pub const PLAYER_INPUT: Self = Self(0x0101);
    pub const WORLD_SNAPSHOT: Self = Self(0x0102);
    pub const DELTA_SNAPSHOT: Self = Self(0x0106);
    pub const CLIENT_INPUT: Self = Self(0x0107);
    pub const GAME_MIN: Self = Self(0x1000);

    pub const fn new(value: u16) -> Self {
        Self(value)
    }

    pub const fn as_u16(self) -> u16 {
        self.0
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum DeliveryClass {
    Unreliable = 0,
    ReliableOrdered = 1,
    ReliableUnordered = 2,
}

impl DeliveryClass {
    fn from_u8(value: u8) -> Result<Self> {
        match value {
            0 => Ok(Self::Unreliable),
            1 => Ok(Self::ReliableOrdered),
            2 => Ok(Self::ReliableUnordered),
            _ => Err(EnservaError::Protocol(format!(
                "unknown delivery class {value}"
            ))),
        }
    }

    fn is_reliable(self) -> bool {
        matches!(self, Self::ReliableOrdered | Self::ReliableUnordered)
    }
}

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct WireCapabilities(u64);

impl WireCapabilities {
    pub const DELTA_SNAPSHOTS: Self = Self(1 << 0);
    pub const RELIABLE_ORDERED: Self = Self(1 << 1);
    pub const RELIABLE_UNORDERED: Self = Self(1 << 2);

    pub const fn all() -> Self {
        Self(Self::DELTA_SNAPSHOTS.0 | Self::RELIABLE_ORDERED.0 | Self::RELIABLE_UNORDERED.0)
    }

    pub const fn bits(self) -> u64 {
        self.0
    }
}

impl std::ops::BitOr for WireCapabilities {
    type Output = WireCapabilities;

    fn bitor(self, rhs: Self) -> Self::Output {
        Self(self.0 | rhs.0)
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct WireMessage {
    pub message_type: WireMessageType,
    pub payload: Vec<u8>,
    pub delivery: DeliveryClass,
    pub reliable_id: u64,
}

impl WireMessage {
    pub fn new(message_type: WireMessageType, payload: Vec<u8>) -> Self {
        Self {
            message_type,
            payload,
            delivery: DeliveryClass::Unreliable,
            reliable_id: 0,
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct WirePacket {
    pub sequence: u64,
    pub ack: u64,
    pub ack_bits: u64,
    pub messages: Vec<WireMessage>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct WelcomeMessage {
    pub client_id: String,
    pub authenticated_id: String,
    pub protocol_version: u8,
    pub capabilities: WireCapabilities,
    pub max_packet_size: u32,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct PongMessage {
    pub nonce: u64,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ErrorMessage {
    pub code: u16,
    pub message: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct DisconnectMessage {
    pub code: u16,
    pub message: String,
}

#[derive(Clone, Debug, PartialEq)]
pub enum SnapshotValue {
    Null,
    Bool(bool),
    Int64(i64),
    Uint64(u64),
    Float64(f64),
    String(String),
    Object(BTreeMap<String, SnapshotValue>),
    List(Vec<SnapshotValue>),
}

pub type SnapshotData = BTreeMap<String, BTreeMap<String, SnapshotValue>>;

#[derive(Clone, Debug, PartialEq)]
pub struct WorldSnapshotMessage {
    pub client_id: String,
    pub tick: u64,
    pub last_sequence: u64,
    pub objects: SnapshotData,
}

#[derive(Clone, Debug, PartialEq)]
pub struct WorldDeltaSnapshotMessage {
    pub client_id: String,
    pub tick: u64,
    pub last_sequence: u64,
    pub baseline_tick: u64,
    pub spawned: SnapshotData,
    pub changed: SnapshotData,
    pub despawned: Vec<SnapshotObjectRef>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct SnapshotObjectRef {
    pub object_type: String,
    pub object_id: String,
}

#[derive(Clone, Debug, PartialEq)]
pub enum ServerMessage {
    Welcome(WelcomeMessage),
    Pong(PongMessage),
    Error(ErrorMessage),
    Disconnect(DisconnectMessage),
    WorldSnapshot(WorldSnapshotMessage),
    WorldDeltaSnapshot(WorldDeltaSnapshotMessage),
    Custom(WireMessage),
}

#[derive(Clone, Debug)]
struct PendingReliableMessage {
    message: WireMessage,
    sent_packet_sequences: BTreeSet<u64>,
    attempts: u32,
    max_attempts: u32,
    retry_interval: Duration,
    next_retry_at: Instant,
}

pub struct EnservaUdpClient {
    socket: UdpSocket,
    next_packet_sequence: u64,
    next_reliable_ordered_id: u64,
    next_reliable_unordered_id: u64,
    latest_peer_sequence: u64,
    received_sequences: HashSet<u64>,
    reliable_ordered_next: u64,
    reliable_ordered_received: HashSet<u64>,
    reliable_unordered_received: HashSet<u64>,
    reliable_ordered_buffer: BTreeMap<u64, WireMessage>,
    pending_reliable: Vec<PendingReliableMessage>,
    received_messages: VecDeque<ServerMessage>,
}

impl EnservaUdpClient {
    pub fn connect<A: ToSocketAddrs>(
        server_addr: A,
        client_name: &str,
        token: &str,
    ) -> Result<Self> {
        let socket = UdpSocket::bind("0.0.0.0:0")?;
        socket.connect(server_addr)?;

        let mut client = Self {
            socket,
            next_packet_sequence: 1,
            next_reliable_ordered_id: 1,
            next_reliable_unordered_id: 1,
            latest_peer_sequence: 0,
            received_sequences: HashSet::new(),
            reliable_ordered_next: 1,
            reliable_ordered_received: HashSet::new(),
            reliable_unordered_received: HashSet::new(),
            reliable_ordered_buffer: BTreeMap::new(),
            pending_reliable: Vec::new(),
            received_messages: VecDeque::new(),
        };
        client.send_client_hello(client_name, token, WireCapabilities::all(), 1200)?;
        Ok(client)
    }

    pub fn set_read_timeout(&self, timeout: Option<Duration>) -> Result<()> {
        self.socket.set_read_timeout(timeout)?;
        Ok(())
    }

    pub fn send_client_hello(
        &mut self,
        client_name: &str,
        token: &str,
        capabilities: WireCapabilities,
        max_packet_size: u32,
    ) -> Result<()> {
        let mut writer = ByteWriter::new();
        writer.write_string(client_name)?;
        writer.write_string(token)?;
        writer.write_u8(PROTOCOL_VERSION);
        writer.write_u64(capabilities.bits());
        writer.write_u32(max_packet_size);
        self.send_message(WireMessage::new(
            WireMessageType::CLIENT_HELLO,
            writer.into_bytes(),
        ))
    }

    pub fn send_ping(&mut self, nonce: u64) -> Result<()> {
        let mut writer = ByteWriter::new();
        writer.write_u64(nonce);
        self.send_message(WireMessage::new(WireMessageType::PING, writer.into_bytes()))
    }

    pub fn send_keep_alive(&mut self) -> Result<()> {
        self.send_ping(0)
    }

    pub fn send_player_input(
        &mut self,
        object_id: &str,
        input_sequence: u64,
        tick: u64,
        x: f32,
        y: f32,
        z: f32,
    ) -> Result<()> {
        let mut writer = ByteWriter::new();
        writer.write_string(object_id)?;
        writer.write_u64(input_sequence);
        writer.write_u64(tick);
        writer.write_f32(x);
        writer.write_f32(y);
        writer.write_f32(z);
        self.send_message(WireMessage::new(
            WireMessageType::PLAYER_INPUT,
            writer.into_bytes(),
        ))
    }

    pub fn send_client_input(
        &mut self,
        input_sequence: u64,
        tick: u64,
        object_type: &str,
        object_id: &str,
        target_id: &str,
        payload: &[u8],
    ) -> Result<()> {
        let mut writer = ByteWriter::new();
        writer.write_u64(input_sequence);
        writer.write_u64(tick);
        writer.write_string(object_type)?;
        writer.write_string(object_id)?;
        writer.write_string(target_id)?;
        writer.write_bytes(payload)?;
        self.send_message(WireMessage::new(
            WireMessageType::CLIENT_INPUT,
            writer.into_bytes(),
        ))
    }

    pub fn send_object_request(
        &mut self,
        object_type: &str,
        object_id: &str,
        action: &str,
        data: &[u8],
        delivery: DeliveryClass,
    ) -> Result<()> {
        let mut writer = ByteWriter::new();
        writer.write_string(object_type)?;
        writer.write_string(object_id)?;
        writer.write_string(action)?;
        writer.write_bytes(data)?;
        let message = self.create_outgoing_message(
            WireMessageType::OBJECT_REQUEST,
            writer.into_bytes(),
            delivery,
        )?;
        self.send_message(message)
    }

    pub fn send_custom_message(
        &mut self,
        message_type: u16,
        payload: &[u8],
        delivery: DeliveryClass,
    ) -> Result<()> {
        if message_type < WireMessageType::GAME_MIN.as_u16() {
            return Err(EnservaError::Protocol(format!(
                "custom game messages must use ids >= 0x1000, got 0x{message_type:04x}"
            )));
        }
        let message = self.create_outgoing_message(
            WireMessageType::new(message_type),
            payload.to_vec(),
            delivery,
        )?;
        self.send_message(message)
    }

    pub fn send_legacy_json_datagram(&self, json: &str) -> Result<()> {
        self.socket.send(json.as_bytes())?;
        Ok(())
    }

    pub fn update(&mut self) -> Result<()> {
        let now = Instant::now();
        let mut retry_messages = Vec::new();

        self.pending_reliable.retain(|pending| {
            if pending.attempts >= pending.max_attempts {
                return false;
            }
            if pending.next_retry_at <= now {
                retry_messages.push(pending.message.clone());
            }
            true
        });

        for message in retry_messages {
            let sequence = self.send_packet(std::slice::from_ref(&message))?;
            if let Some(pending) = self
                .pending_reliable
                .iter_mut()
                .find(|pending| pending.message.reliable_id == message.reliable_id)
            {
                pending.attempts += 1;
                pending.sent_packet_sequences.insert(sequence);
                pending.next_retry_at = Instant::now() + pending.retry_interval;
            }
        }

        Ok(())
    }

    pub fn receive_one(&mut self) -> Result<Option<ServerMessage>> {
        if let Some(message) = self.received_messages.pop_front() {
            return Ok(Some(message));
        }

        let mut buffer = [0u8; 65_507];
        match self.socket.recv(&mut buffer) {
            Ok(length) => self.handle_datagram(&buffer[..length]),
            Err(error)
                if matches!(
                    error.kind(),
                    io::ErrorKind::WouldBlock | io::ErrorKind::TimedOut
                ) =>
            {
                Ok(None)
            }
            Err(error) => Err(error.into()),
        }
    }

    pub fn handle_datagram(&mut self, datagram: &[u8]) -> Result<Option<ServerMessage>> {
        if !looks_like_wire_packet(datagram) {
            return Ok(None);
        }

        let packet = decode_packet(datagram)?;
        self.record_peer_packet(packet.sequence);
        self.remove_acked_reliable_messages(packet.ack, packet.ack_bits);

        let accepted_messages = self.accept_incoming_reliable_messages(packet.messages);
        for message in accepted_messages {
            self.received_messages
                .push_back(decode_server_message(message)?);
        }

        Ok(self.received_messages.pop_front())
    }

    fn send_message(&mut self, message: WireMessage) -> Result<()> {
        let reliable = message.delivery.is_reliable();
        let sequence = self.send_packet(std::slice::from_ref(&message))?;
        if reliable {
            self.track_reliable_message(message, sequence);
        }
        Ok(())
    }

    fn send_packet(&mut self, messages: &[WireMessage]) -> Result<u64> {
        let sequence = self.next_packet_sequence;
        self.next_packet_sequence += 1;

        let ack = self.latest_peer_sequence;
        let ack_bits = self.build_ack_bits();
        let packet = encode_packet(sequence, ack, ack_bits, messages)?;
        self.socket.send(&packet)?;
        Ok(sequence)
    }

    fn create_outgoing_message(
        &mut self,
        message_type: WireMessageType,
        payload: Vec<u8>,
        delivery: DeliveryClass,
    ) -> Result<WireMessage> {
        let reliable_id = match delivery {
            DeliveryClass::Unreliable => 0,
            DeliveryClass::ReliableOrdered => {
                let id = self.next_reliable_ordered_id;
                self.next_reliable_ordered_id += 1;
                id
            }
            DeliveryClass::ReliableUnordered => {
                let id = self.next_reliable_unordered_id;
                self.next_reliable_unordered_id += 1;
                id
            }
        };

        Ok(WireMessage {
            message_type,
            payload,
            delivery,
            reliable_id,
        })
    }

    fn track_reliable_message(&mut self, message: WireMessage, packet_sequence: u64) {
        let mut sent_packet_sequences = BTreeSet::new();
        sent_packet_sequences.insert(packet_sequence);
        self.pending_reliable.push(PendingReliableMessage {
            message,
            sent_packet_sequences,
            attempts: 1,
            max_attempts: 5,
            retry_interval: Duration::from_millis(100),
            next_retry_at: Instant::now() + Duration::from_millis(100),
        });
    }

    fn remove_acked_reliable_messages(&mut self, ack: u64, ack_bits: u64) {
        self.pending_reliable.retain(|pending| {
            !pending
                .sent_packet_sequences
                .iter()
                .any(|sequence| packet_sequence_acked(ack, ack_bits, *sequence))
        });
    }

    fn record_peer_packet(&mut self, sequence: u64) {
        if sequence == 0 {
            return;
        }

        self.received_sequences.insert(sequence);
        if sequence > self.latest_peer_sequence {
            self.latest_peer_sequence = sequence;
        }
        if self.latest_peer_sequence > 64 {
            let minimum = self.latest_peer_sequence - 64;
            self.received_sequences
                .retain(|sequence| *sequence >= minimum);
        }
    }

    fn build_ack_bits(&self) -> u64 {
        if self.latest_peer_sequence == 0 {
            return 0;
        }

        let mut bits = 0u64;
        for offset in 0..64 {
            let sequence = self.latest_peer_sequence.saturating_sub(offset + 1);
            if self.received_sequences.contains(&sequence) {
                bits |= 1u64 << offset;
            }
            if sequence == 0 {
                break;
            }
        }
        bits
    }

    fn accept_incoming_reliable_messages(
        &mut self,
        messages: Vec<WireMessage>,
    ) -> Vec<WireMessage> {
        let mut accepted = Vec::new();

        for message in messages {
            if message.delivery == DeliveryClass::Unreliable {
                accepted.push(message);
                continue;
            }
            if message.reliable_id == 0 {
                continue;
            }

            match message.delivery {
                DeliveryClass::ReliableUnordered => {
                    if self.reliable_unordered_received.insert(message.reliable_id) {
                        accepted.push(message);
                    }
                }
                DeliveryClass::ReliableOrdered => {
                    if message.reliable_id < self.reliable_ordered_next
                        || self
                            .reliable_ordered_received
                            .contains(&message.reliable_id)
                        || self
                            .reliable_ordered_buffer
                            .contains_key(&message.reliable_id)
                    {
                        continue;
                    }

                    if message.reliable_id > self.reliable_ordered_next {
                        self.reliable_ordered_buffer
                            .insert(message.reliable_id, message);
                        continue;
                    }

                    self.accept_ordered_message(message, &mut accepted);
                }
                DeliveryClass::Unreliable => {}
            }
        }

        accepted
    }

    fn accept_ordered_message(&mut self, message: WireMessage, accepted: &mut Vec<WireMessage>) {
        accepted.push(message.clone());
        self.reliable_ordered_received.insert(message.reliable_id);
        self.reliable_ordered_next += 1;

        while let Some(buffered) = self
            .reliable_ordered_buffer
            .remove(&self.reliable_ordered_next)
        {
            accepted.push(buffered.clone());
            self.reliable_ordered_received.insert(buffered.reliable_id);
            self.reliable_ordered_next += 1;
        }
    }
}

pub fn encode_packet(
    sequence: u64,
    ack: u64,
    ack_bits: u64,
    messages: &[WireMessage],
) -> Result<Vec<u8>> {
    if messages.is_empty() || messages.len() > MAX_MESSAGES_PER_PACKET {
        return Err(EnservaError::Protocol(format!(
            "Enserva packets must contain 1..32 messages, got {}",
            messages.len()
        )));
    }

    let mut payload = ByteWriter::new();
    for message in messages {
        let mut wire_type = message.message_type;
        let mut wire_payload = message.payload.clone();

        if message.delivery.is_reliable() {
            wire_type = WireMessageType::RELIABLE;
            wire_payload = encode_reliable_envelope(message)?;
        }

        if wire_payload.len() > MAX_WIRE_MESSAGE_PAYLOAD_SIZE {
            return Err(EnservaError::Protocol(format!(
                "wire message payload is too large: {}",
                wire_payload.len()
            )));
        }

        payload.write_u16(wire_type.as_u16());
        payload.write_u32(wire_payload.len() as u32);
        payload.write_raw(&wire_payload);
    }

    let framed_messages = payload.into_bytes();
    let mut packet = ByteWriter::new();
    packet.write_u16(MAGIC);
    packet.write_u8(PROTOCOL_VERSION);
    packet.write_u8(messages.len() as u8);
    packet.write_u32(0);
    packet.write_u64(sequence);
    packet.write_u64(ack);
    packet.write_u64(ack_bits);
    packet.write_u32(framed_messages.len() as u32);
    packet.write_raw(&framed_messages);
    Ok(packet.into_bytes())
}

pub fn decode_packet(data: &[u8]) -> Result<WirePacket> {
    if data.len() < PACKET_HEADER_SIZE {
        return Err(EnservaError::Protocol(
            "wire packet header is too short".to_string(),
        ));
    }

    let mut reader = ByteReader::new(data);
    let magic = reader.read_u16()?;
    if magic != MAGIC {
        return Err(EnservaError::Protocol("bad Enserva wire magic".to_string()));
    }

    let version = reader.read_u8()?;
    if version != PROTOCOL_VERSION {
        return Err(EnservaError::Protocol(format!(
            "unsupported Enserva wire protocol version: {version}"
        )));
    }

    let message_count = reader.read_u8()? as usize;
    if message_count == 0 || message_count > MAX_MESSAGES_PER_PACKET {
        return Err(EnservaError::Protocol(format!(
            "invalid Enserva message count: {message_count}"
        )));
    }

    reader.read_u32()?;
    let sequence = reader.read_u64()?;
    let ack = reader.read_u64()?;
    let ack_bits = reader.read_u64()?;
    let payload_length = reader.read_u32()? as usize;
    if payload_length != data.len() - PACKET_HEADER_SIZE {
        return Err(EnservaError::Protocol(
            "wire packet payload length mismatch".to_string(),
        ));
    }

    let mut messages = Vec::with_capacity(message_count);
    for _ in 0..message_count {
        if reader.remaining() < MESSAGE_HEADER_SIZE {
            return Err(EnservaError::Protocol(
                "wire message header is truncated".to_string(),
            ));
        }
        let message_type = WireMessageType::new(reader.read_u16()?);
        let length = reader.read_u32()? as usize;
        if length > MAX_WIRE_MESSAGE_PAYLOAD_SIZE {
            return Err(EnservaError::Protocol(format!(
                "wire message payload is too large: {length}"
            )));
        }
        let payload = reader.read_exact(length)?;
        messages.push(if message_type == WireMessageType::RELIABLE {
            decode_reliable_envelope(&payload)?
        } else {
            WireMessage::new(message_type, payload)
        });
    }

    if reader.remaining() != 0 {
        return Err(EnservaError::Protocol(
            "wire packet has trailing bytes".to_string(),
        ));
    }

    Ok(WirePacket {
        sequence,
        ack,
        ack_bits,
        messages,
    })
}

fn encode_reliable_envelope(message: &WireMessage) -> Result<Vec<u8>> {
    if message.reliable_id == 0 {
        return Err(EnservaError::Protocol(
            "reliable messages require a non-zero reliable id".to_string(),
        ));
    }

    let mut writer = ByteWriter::new();
    writer.write_u8(message.delivery as u8);
    writer.write_u64(message.reliable_id);
    writer.write_u16(message.message_type.as_u16());
    writer.write_u32(message.payload.len() as u32);
    writer.write_raw(&message.payload);
    Ok(writer.into_bytes())
}

fn decode_reliable_envelope(payload: &[u8]) -> Result<WireMessage> {
    let mut reader = ByteReader::new(payload);
    let delivery = DeliveryClass::from_u8(reader.read_u8()?)?;
    if !delivery.is_reliable() {
        return Err(EnservaError::Protocol(format!(
            "invalid reliable delivery class: {}",
            delivery as u8
        )));
    }
    let reliable_id = reader.read_u64()?;
    let message_type = WireMessageType::new(reader.read_u16()?);
    let inner_length = reader.read_u32()? as usize;
    let inner_payload = reader.read_exact(inner_length)?;
    if reader.remaining() != 0 {
        return Err(EnservaError::Protocol(
            "reliable envelope has trailing bytes".to_string(),
        ));
    }
    Ok(WireMessage {
        message_type,
        payload: inner_payload,
        delivery,
        reliable_id,
    })
}

pub fn decode_server_message(message: WireMessage) -> Result<ServerMessage> {
    let mut reader = ByteReader::new(&message.payload);
    match message.message_type {
        WireMessageType::WELCOME => Ok(ServerMessage::Welcome(decode_welcome(&mut reader)?)),
        WireMessageType::PONG => Ok(ServerMessage::Pong(PongMessage {
            nonce: reader.read_u64()?,
        })),
        WireMessageType::ERROR => Ok(ServerMessage::Error(decode_code_message(&mut reader)?)),
        WireMessageType::DISCONNECT => {
            let code_message = decode_code_message(&mut reader)?;
            Ok(ServerMessage::Disconnect(DisconnectMessage {
                code: code_message.code,
                message: code_message.message,
            }))
        }
        WireMessageType::WORLD_SNAPSHOT => Ok(ServerMessage::WorldSnapshot(decode_world_snapshot(
            &mut reader,
        )?)),
        WireMessageType::DELTA_SNAPSHOT => Ok(ServerMessage::WorldDeltaSnapshot(
            decode_world_delta_snapshot(&mut reader)?,
        )),
        _ => Ok(ServerMessage::Custom(message)),
    }
}

fn decode_welcome(reader: &mut ByteReader<'_>) -> Result<WelcomeMessage> {
    let client_id = reader.read_string()?;
    let authenticated_id = reader.read_string()?;
    let mut protocol_version = 0;
    let mut capabilities = WireCapabilities::default();
    let mut max_packet_size = 0;

    if reader.remaining() > 0 {
        if reader.remaining() < 13 {
            return Err(EnservaError::Protocol(
                "welcome negotiation data is truncated".to_string(),
            ));
        }
        protocol_version = reader.read_u8()?;
        capabilities = WireCapabilities(reader.read_u64()?);
        max_packet_size = reader.read_u32()?;
    }

    Ok(WelcomeMessage {
        client_id,
        authenticated_id,
        protocol_version,
        capabilities,
        max_packet_size,
    })
}

fn decode_code_message(reader: &mut ByteReader<'_>) -> Result<ErrorMessage> {
    Ok(ErrorMessage {
        code: reader.read_u16()?,
        message: reader.read_string()?,
    })
}

fn decode_world_snapshot(reader: &mut ByteReader<'_>) -> Result<WorldSnapshotMessage> {
    Ok(WorldSnapshotMessage {
        client_id: reader.read_string()?,
        tick: reader.read_u64()?,
        last_sequence: reader.read_u64()?,
        objects: reader.read_snapshot_data()?,
    })
}

fn decode_world_delta_snapshot(reader: &mut ByteReader<'_>) -> Result<WorldDeltaSnapshotMessage> {
    Ok(WorldDeltaSnapshotMessage {
        client_id: reader.read_string()?,
        tick: reader.read_u64()?,
        last_sequence: reader.read_u64()?,
        baseline_tick: reader.read_u64()?,
        spawned: reader.read_snapshot_data()?,
        changed: reader.read_snapshot_data()?,
        despawned: reader.read_snapshot_object_refs()?,
    })
}

fn packet_sequence_acked(ack: u64, ack_bits: u64, sequence: u64) -> bool {
    if sequence == 0 || ack == 0 {
        return false;
    }
    if sequence == ack {
        return true;
    }
    if sequence > ack {
        return false;
    }

    let distance = ack - sequence;
    if distance == 0 || distance > 64 {
        return false;
    }

    (ack_bits & (1u64 << (distance - 1))) != 0
}

fn looks_like_wire_packet(payload: &[u8]) -> bool {
    payload.len() >= 2 && payload[0] == (MAGIC >> 8) as u8 && payload[1] == (MAGIC & 0xff) as u8
}

struct ByteWriter {
    bytes: Vec<u8>,
}

impl ByteWriter {
    fn new() -> Self {
        Self { bytes: Vec::new() }
    }

    fn write_u8(&mut self, value: u8) {
        self.bytes.push(value);
    }

    fn write_u16(&mut self, value: u16) {
        self.bytes.extend_from_slice(&value.to_be_bytes());
    }

    fn write_u32(&mut self, value: u32) {
        self.bytes.extend_from_slice(&value.to_be_bytes());
    }

    fn write_u64(&mut self, value: u64) {
        self.bytes.extend_from_slice(&value.to_be_bytes());
    }

    fn write_f32(&mut self, value: f32) {
        self.write_u32(value.to_bits());
    }

    fn write_string(&mut self, value: &str) -> Result<()> {
        let text = value.as_bytes();
        if text.len() > MAX_WIRE_STRING_BYTES || text.len() > u16::MAX as usize {
            return Err(EnservaError::Protocol(format!(
                "wire string is too large: {}",
                text.len()
            )));
        }
        self.write_u16(text.len() as u16);
        self.bytes.extend_from_slice(text);
        Ok(())
    }

    fn write_bytes(&mut self, value: &[u8]) -> Result<()> {
        if value.len() > u32::MAX as usize {
            return Err(EnservaError::Protocol(format!(
                "byte payload is too large: {}",
                value.len()
            )));
        }
        self.write_u32(value.len() as u32);
        self.bytes.extend_from_slice(value);
        Ok(())
    }

    fn write_raw(&mut self, value: &[u8]) {
        self.bytes.extend_from_slice(value);
    }

    fn into_bytes(self) -> Vec<u8> {
        self.bytes
    }
}

struct ByteReader<'a> {
    bytes: &'a [u8],
    position: usize,
}

impl<'a> ByteReader<'a> {
    fn new(bytes: &'a [u8]) -> Self {
        Self { bytes, position: 0 }
    }

    fn remaining(&self) -> usize {
        self.bytes.len() - self.position
    }

    fn read_u8(&mut self) -> Result<u8> {
        let value = self.read_exact(1)?[0];
        Ok(value)
    }

    fn read_u16(&mut self) -> Result<u16> {
        let data = self.read_exact(2)?;
        Ok(u16::from_be_bytes([data[0], data[1]]))
    }

    fn read_u32(&mut self) -> Result<u32> {
        let data = self.read_exact(4)?;
        Ok(u32::from_be_bytes([data[0], data[1], data[2], data[3]]))
    }

    fn read_u64(&mut self) -> Result<u64> {
        let data = self.read_exact(8)?;
        Ok(u64::from_be_bytes([
            data[0], data[1], data[2], data[3], data[4], data[5], data[6], data[7],
        ]))
    }

    fn read_f64(&mut self) -> Result<f64> {
        Ok(f64::from_bits(self.read_u64()?))
    }

    fn read_string(&mut self) -> Result<String> {
        let length = self.read_u16()? as usize;
        if length > MAX_WIRE_STRING_BYTES {
            return Err(EnservaError::Protocol(format!(
                "wire string is too large: {length}"
            )));
        }
        let data = self.read_exact(length)?;
        Ok(String::from_utf8(data)?)
    }

    fn read_exact(&mut self, length: usize) -> Result<Vec<u8>> {
        if self.remaining() < length {
            return Err(EnservaError::Protocol(
                "wire payload is truncated".to_string(),
            ));
        }
        let value = self.bytes[self.position..self.position + length].to_vec();
        self.position += length;
        Ok(value)
    }

    fn read_snapshot_data(&mut self) -> Result<SnapshotData> {
        let type_count = self.read_u16()? as usize;
        let mut snapshot = SnapshotData::new();

        for _ in 0..type_count {
            let object_type = self.read_string()?;
            let object_count = self.read_u16()? as usize;
            let mut objects_by_id = BTreeMap::new();

            for _ in 0..object_count {
                let object_id = self.read_string()?;
                objects_by_id.insert(object_id, self.read_wire_value(0)?);
            }

            snapshot.insert(object_type, objects_by_id);
        }

        Ok(snapshot)
    }

    fn read_snapshot_object_refs(&mut self) -> Result<Vec<SnapshotObjectRef>> {
        let count = self.read_u16()? as usize;
        let mut refs = Vec::with_capacity(count);

        for _ in 0..count {
            refs.push(SnapshotObjectRef {
                object_type: self.read_string()?,
                object_id: self.read_string()?,
            });
        }

        Ok(refs)
    }

    fn read_wire_value(&mut self, depth: u8) -> Result<SnapshotValue> {
        if depth > 16 {
            return Err(EnservaError::Protocol(
                "wire value nesting is too deep".to_string(),
            ));
        }

        match self.read_u8()? {
            0 => Ok(SnapshotValue::Null),
            1 => Ok(SnapshotValue::Bool(self.read_u8()? != 0)),
            2 => Ok(SnapshotValue::Int64(self.read_u64()? as i64)),
            3 => Ok(SnapshotValue::Uint64(self.read_u64()?)),
            4 => Ok(SnapshotValue::Float64(self.read_f64()?)),
            5 => Ok(SnapshotValue::String(self.read_string()?)),
            6 => {
                let count = self.read_u16()? as usize;
                let mut object = BTreeMap::new();
                for _ in 0..count {
                    let key = self.read_string()?;
                    object.insert(key, self.read_wire_value(depth + 1)?);
                }
                Ok(SnapshotValue::Object(object))
            }
            7 => {
                let count = self.read_u16()? as usize;
                let mut values = Vec::with_capacity(count);
                for _ in 0..count {
                    values.push(self.read_wire_value(depth + 1)?);
                }
                Ok(SnapshotValue::List(values))
            }
            kind => Err(EnservaError::Protocol(format!(
                "unknown wire value kind: {kind}"
            ))),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn encodes_and_decodes_player_input_packet() {
        let mut writer = ByteWriter::new();
        writer.write_string("player-1").unwrap();
        writer.write_u64(1);
        writer.write_u64(120);
        writer.write_f32(1.0);
        writer.write_f32(0.0);
        writer.write_f32(0.0);

        let message = WireMessage::new(WireMessageType::PLAYER_INPUT, writer.into_bytes());
        let packet = encode_packet(7, 3, 0b10, &[message]).unwrap();

        assert_eq!(&packet[0..2], &[0x45, 0x53]);
        assert_eq!(packet[2], PROTOCOL_VERSION);
        assert_eq!(packet[3], 1);
        assert_eq!(u64::from_be_bytes(packet[8..16].try_into().unwrap()), 7);
        assert_eq!(u64::from_be_bytes(packet[16..24].try_into().unwrap()), 3);
        assert_eq!(u64::from_be_bytes(packet[24..32].try_into().unwrap()), 0b10);

        let decoded = decode_packet(&packet).unwrap();
        assert_eq!(decoded.sequence, 7);
        assert_eq!(decoded.ack, 3);
        assert_eq!(decoded.ack_bits, 0b10);
        assert_eq!(decoded.messages.len(), 1);
        assert_eq!(
            decoded.messages[0].message_type,
            WireMessageType::PLAYER_INPUT
        );

        let mut payload = ByteReader::new(&decoded.messages[0].payload);
        assert_eq!(payload.read_string().unwrap(), "player-1");
        assert_eq!(payload.read_u64().unwrap(), 1);
        assert_eq!(payload.read_u64().unwrap(), 120);
        assert_eq!(
            f32::from_bits(payload.read_u32().unwrap()).to_bits(),
            1.0f32.to_bits()
        );
    }

    #[test]
    fn encodes_reliable_envelope() {
        let message = WireMessage {
            message_type: WireMessageType::new(0x1001),
            payload: vec![1, 2, 3],
            delivery: DeliveryClass::ReliableOrdered,
            reliable_id: 9,
        };

        let packet = encode_packet(1, 0, 0, &[message]).unwrap();
        let decoded = decode_packet(&packet).unwrap();

        assert_eq!(decoded.messages.len(), 1);
        assert_eq!(
            decoded.messages[0].message_type,
            WireMessageType::new(0x1001)
        );
        assert_eq!(decoded.messages[0].payload, vec![1, 2, 3]);
        assert_eq!(decoded.messages[0].delivery, DeliveryClass::ReliableOrdered);
        assert_eq!(decoded.messages[0].reliable_id, 9);
    }

    #[test]
    fn decodes_welcome_message() {
        let mut writer = ByteWriter::new();
        writer.write_string("connection-1").unwrap();
        writer.write_string("account-1").unwrap();
        writer.write_u8(PROTOCOL_VERSION);
        writer.write_u64(WireCapabilities::all().bits());
        writer.write_u32(1200);

        let server_message = decode_server_message(WireMessage::new(
            WireMessageType::WELCOME,
            writer.into_bytes(),
        ))
        .unwrap();

        match server_message {
            ServerMessage::Welcome(welcome) => {
                assert_eq!(welcome.client_id, "connection-1");
                assert_eq!(welcome.authenticated_id, "account-1");
                assert_eq!(welcome.protocol_version, PROTOCOL_VERSION);
                assert_eq!(welcome.capabilities, WireCapabilities::all());
                assert_eq!(welcome.max_packet_size, 1200);
            }
            other => panic!("expected welcome, got {other:?}"),
        }
    }

    #[test]
    fn packet_ack_bits_match_protocol_rules() {
        assert!(packet_sequence_acked(10, 0, 10));
        assert!(packet_sequence_acked(10, 1 << 0, 9));
        assert!(packet_sequence_acked(10, 1 << 3, 6));
        assert!(!packet_sequence_acked(10, 1 << 3, 5));
        assert!(!packet_sequence_acked(10, 0, 11));
    }

    #[test]
    fn rejects_oversized_strings() {
        let mut writer = ByteWriter::new();
        let value = "x".repeat(MAX_WIRE_STRING_BYTES + 1);
        assert!(writer.write_string(&value).is_err());
    }
}
