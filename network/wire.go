package network

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

const (
	// WireProtocolMagic identifies Enserva binary UDP packets: ASCII "ES".
	WireProtocolMagic uint16 = 0x4553
	// WireProtocolVersion is bumped when the binary packet format changes.
	WireProtocolVersion uint8 = 1

	wirePacketHeaderSize  = 36
	wireMessageHeaderSize = 6

	// MaxWirePayloadSize keeps UDP packets bounded below the IPv4 UDP payload limit.
	MaxWirePayloadSize = 60 * 1024
	// MaxWireMessagePayloadSize prevents one message from consuming an entire datagram unexpectedly.
	MaxWireMessagePayloadSize = 48 * 1024
	MaxWireMessagesPerPacket  = 32
	MaxWireStringBytes        = 2048
	MaxWireChatBytes          = 512
)

var (
	ErrInvalidWirePacket            = errors.New("invalid wire packet")
	ErrUnsupportedWireVersion       = errors.New("unsupported wire protocol version")
	ErrWirePacketTooLarge           = errors.New("wire packet too large")
	ErrWireMessageTooLarge          = errors.New("wire message too large")
	ErrWireStringTooLarge           = errors.New("wire string too large")
	ErrMalformedWirePayload         = errors.New("malformed wire payload")
	ErrUnsupportedWireMessage       = errors.New("unsupported wire message")
	ErrUnsupportedWireSnapshotValue = errors.New("unsupported wire snapshot value")
)

// Wire format:
//
// Packet header, big endian:
//   uint16 magic              0x4553 ("ES")
//   uint8  protocol_version   currently 1
//   uint8  message_count      1..32 messages
//   uint32 reserved           reserved for future flags/capabilities
//   uint64 sequence_number    transport-level client sequence or server tick/sequence
//   uint64 ack                latest peer sequence received by sender
//   uint64 ack_bits           bit N acks ack-(N+1), for the previous 64 sequences
//   uint32 payload_length     bytes following the packet header
//
// Message header, repeated message_count times:
//   uint16 message_type       stable registry ID
//   uint32 payload_length
//   bytes  payload
//
// Unknown message types are preserved as UnknownWireMessage so callers can skip them
// without dropping the whole packet. Malformed, oversized, or wrong-version packets fail.

// WireMessageType identifies the schema used for a message payload.
type WireMessageType uint16

const (
	WireMessageUnknown WireMessageType = 0

	// Protocol/system messages.
	WireMessageClientHello WireMessageType = 0x0001
	WireMessageWelcome     WireMessageType = 0x0002
	WireMessagePing        WireMessageType = 0x0003
	WireMessagePong        WireMessageType = 0x0004
	WireMessageError       WireMessageType = 0x0005
	WireMessageDisconnect  WireMessageType = 0x0006

	// Built-in engine messages. Games may ignore these and register their own
	// messages in the game range instead.
	WireMessageObjectRequest WireMessageType = 0x0100
	WireMessagePlayerInput   WireMessageType = 0x0101
	WireMessageWorldSnapshot WireMessageType = 0x0102
	WireMessageEntitySpawn   WireMessageType = 0x0103
	WireMessageEntityDespawn WireMessageType = 0x0104
	WireMessageEntityUpdate  WireMessageType = 0x0105
)

// WireMessage is a framed message with an already encoded payload.
type WireMessage struct {
	Type    WireMessageType
	Payload []byte
}

// WirePacket is a decoded binary packet.
type WirePacket struct {
	Version  uint8
	Sequence uint64
	Ack      uint64
	AckBits  uint64
	Messages []WireMessage
}

// UnknownWireMessage preserves message types this build does not understand.
type UnknownWireMessage struct {
	Type    WireMessageType
	Payload []byte
}

// ClientHello is the binary handshake/authentication message.
type ClientHello struct {
	ClientName string
	Token      string
}

// ObjectRequest is the generic compatibility request. Data is an opaque bounded
// payload, usually legacy JSON for object handlers that have not moved to typed messages.
type ObjectRequest struct {
	ObjectType string
	ObjectID   string
	Action     string
	Data       []byte
}

// PlayerInput is the hot-path built-in player movement input.
type PlayerInput struct {
	ObjectID string
	X        float32
	Y        float32
	Z        float32
}

// Ping carries an application nonce.
type Ping struct {
	Nonce uint64
}

// Pong echoes an application nonce.
type Pong struct {
	Nonce uint64
}

// ChatMessage carries small non-gameplay text messages.
type ChatMessage struct {
	Text string
}

// Welcome accepts a handshake/authentication attempt.
type Welcome struct {
	ClientID        string
	AuthenticatedID string
}

// WorldSnapshot publishes the current visible world state for one client.
type WorldSnapshot struct {
	ClientID     string
	Tick         uint64
	LastSequence uint64
	Objects      SnapshotData
}

// EntitySpawn is reserved for future delta replication.
type EntitySpawn struct {
	ObjectType string
	ObjectID   string
	Snapshot   any
}

// EntityDespawn is reserved for future delta replication.
type EntityDespawn struct {
	ObjectType string
	ObjectID   string
}

// EntityUpdate is reserved for future delta replication.
type EntityUpdate struct {
	ObjectType string
	ObjectID   string
	Snapshot   any
}

// ErrorMessage reports a protocol or request error.
type ErrorMessage struct {
	Code    uint16
	Message string
}

// DisconnectMessage reports why a peer should stop using the session.
type DisconnectMessage struct {
	Code    uint16
	Message string
}

// EncodePacket frames one or more encoded messages into a binary wire packet.
func EncodePacket(sequence uint64, messages []WireMessage) ([]byte, error) {
	return EncodePacketWithAcks(sequence, 0, 0, messages)
}

// EncodePacketWithAcks frames messages and includes sender-side acknowledgement
// state for unreliable transports such as UDP.
func EncodePacketWithAcks(sequence, ack, ackBits uint64, messages []WireMessage) ([]byte, error) {
	if len(messages) == 0 || len(messages) > MaxWireMessagesPerPacket {
		return nil, fmt.Errorf("%w: message count %d", ErrInvalidWirePacket, len(messages))
	}

	payload := bytes.Buffer{}
	for _, message := range messages {
		if len(message.Payload) > MaxWireMessagePayloadSize {
			return nil, fmt.Errorf("%w: %d", ErrWireMessageTooLarge, len(message.Payload))
		}
		writeUint16(&payload, uint16(message.Type))
		writeUint32(&payload, uint32(len(message.Payload)))
		payload.Write(message.Payload)
	}
	if payload.Len() > MaxWirePayloadSize {
		return nil, fmt.Errorf("%w: %d", ErrWirePacketTooLarge, payload.Len())
	}

	packet := bytes.Buffer{}
	writeUint16(&packet, WireProtocolMagic)
	packet.WriteByte(WireProtocolVersion)
	packet.WriteByte(byte(len(messages)))
	writeUint32(&packet, 0)
	writeUint64(&packet, sequence)
	writeUint64(&packet, ack)
	writeUint64(&packet, ackBits)
	writeUint32(&packet, uint32(payload.Len()))
	packet.Write(payload.Bytes())

	return packet.Bytes(), nil
}

// DecodePacket validates and splits a binary wire packet into messages.
func DecodePacket(data []byte) (WirePacket, error) {
	if len(data) < wirePacketHeaderSize {
		return WirePacket{}, fmt.Errorf("%w: header too short", ErrInvalidWirePacket)
	}
	if len(data)-wirePacketHeaderSize > MaxWirePayloadSize {
		return WirePacket{}, fmt.Errorf("%w: %d", ErrWirePacketTooLarge, len(data)-wirePacketHeaderSize)
	}

	reader := bytes.NewReader(data)
	magic, _ := readUint16(reader)
	if magic != WireProtocolMagic {
		return WirePacket{}, fmt.Errorf("%w: bad magic", ErrInvalidWirePacket)
	}

	version, _ := reader.ReadByte()
	if version != WireProtocolVersion {
		return WirePacket{}, fmt.Errorf("%w: %d", ErrUnsupportedWireVersion, version)
	}

	messageCountByte, _ := reader.ReadByte()
	messageCount := int(messageCountByte)
	if messageCount == 0 || messageCount > MaxWireMessagesPerPacket {
		return WirePacket{}, fmt.Errorf("%w: message count %d", ErrInvalidWirePacket, messageCount)
	}

	if _, err := readUint32(reader); err != nil {
		return WirePacket{}, err
	}
	sequence, _ := readUint64(reader)
	ack, _ := readUint64(reader)
	ackBits, _ := readUint64(reader)
	payloadLength, _ := readUint32(reader)
	if int(payloadLength) != len(data)-wirePacketHeaderSize {
		return WirePacket{}, fmt.Errorf("%w: payload length mismatch", ErrInvalidWirePacket)
	}

	messages := make([]WireMessage, 0, messageCount)
	for index := 0; index < messageCount; index++ {
		if reader.Len() < wireMessageHeaderSize {
			return WirePacket{}, fmt.Errorf("%w: message header too short", ErrInvalidWirePacket)
		}

		messageType, _ := readUint16(reader)
		messageLength, _ := readUint32(reader)
		if messageLength > MaxWireMessagePayloadSize {
			return WirePacket{}, fmt.Errorf("%w: %d", ErrWireMessageTooLarge, messageLength)
		}
		if int(messageLength) > reader.Len() {
			return WirePacket{}, fmt.Errorf("%w: message payload truncated", ErrInvalidWirePacket)
		}

		payload := make([]byte, int(messageLength))
		if _, err := reader.Read(payload); err != nil {
			return WirePacket{}, err
		}
		messages = append(messages, WireMessage{
			Type:    WireMessageType(messageType),
			Payload: payload,
		})
	}
	if reader.Len() != 0 {
		return WirePacket{}, fmt.Errorf("%w: trailing bytes", ErrInvalidWirePacket)
	}

	return WirePacket{
		Version:  version,
		Sequence: sequence,
		Ack:      ack,
		AckBits:  ackBits,
		Messages: messages,
	}, nil
}

// DecodeClientMessage converts one framed client message into a typed schema.
func DecodeClientMessage(message WireMessage) (any, error) {
	return DefaultWireMessages().DecodeMessage(message)
}

// EncodeClientMessage encodes a typed client message for tests, tools, and clients.
func EncodeClientMessage(message any) (WireMessage, error) {
	return DefaultWireMessages().EncodeMessage(message)
}

// DecodeServerMessage converts one framed server message into a typed schema.
func DecodeServerMessage(message WireMessage) (any, error) {
	return DefaultWireMessages().DecodeMessage(message)
}

// EncodeServerMessage encodes a typed server message.
func EncodeServerMessage(message any) (WireMessage, error) {
	return DefaultWireMessages().EncodeMessage(message)
}

func EncodeClientHello(message ClientHello) ([]byte, error) {
	writer := bytes.Buffer{}
	if err := writeString(&writer, message.ClientName, MaxWireStringBytes); err != nil {
		return nil, err
	}
	if err := writeString(&writer, message.Token, MaxWireStringBytes); err != nil {
		return nil, err
	}
	return writer.Bytes(), nil
}

func DecodeClientHello(payload []byte) (ClientHello, error) {
	reader := bytes.NewReader(payload)
	clientName, err := readString(reader, MaxWireStringBytes)
	if err != nil {
		return ClientHello{}, err
	}
	token, err := readString(reader, MaxWireStringBytes)
	if err != nil {
		return ClientHello{}, err
	}
	if reader.Len() != 0 {
		return ClientHello{}, fmt.Errorf("%w: trailing hello bytes", ErrMalformedWirePayload)
	}
	return ClientHello{ClientName: clientName, Token: token}, nil
}

func EncodeObjectRequest(message ObjectRequest) ([]byte, error) {
	if len(message.Data) > MaxWireMessagePayloadSize {
		return nil, fmt.Errorf("%w: object request data %d", ErrWireMessageTooLarge, len(message.Data))
	}
	writer := bytes.Buffer{}
	if err := writeString(&writer, message.ObjectType, MaxWireStringBytes); err != nil {
		return nil, err
	}
	if err := writeString(&writer, message.ObjectID, MaxWireStringBytes); err != nil {
		return nil, err
	}
	if err := writeString(&writer, message.Action, MaxWireStringBytes); err != nil {
		return nil, err
	}
	writeBytes(&writer, message.Data)
	return writer.Bytes(), nil
}

func DecodeObjectRequest(payload []byte) (ObjectRequest, error) {
	reader := bytes.NewReader(payload)
	objectType, err := readString(reader, MaxWireStringBytes)
	if err != nil {
		return ObjectRequest{}, err
	}
	objectID, err := readString(reader, MaxWireStringBytes)
	if err != nil {
		return ObjectRequest{}, err
	}
	action, err := readString(reader, MaxWireStringBytes)
	if err != nil {
		return ObjectRequest{}, err
	}
	data, err := readBytes(reader, MaxWireMessagePayloadSize)
	if err != nil {
		return ObjectRequest{}, err
	}
	if reader.Len() != 0 {
		return ObjectRequest{}, fmt.Errorf("%w: trailing object request bytes", ErrMalformedWirePayload)
	}
	return ObjectRequest{
		ObjectType: objectType,
		ObjectID:   objectID,
		Action:     action,
		Data:       data,
	}, nil
}

func EncodePlayerInput(message PlayerInput) ([]byte, error) {
	writer := bytes.Buffer{}
	if err := writeString(&writer, message.ObjectID, MaxWireStringBytes); err != nil {
		return nil, err
	}
	writeFloat32(&writer, message.X)
	writeFloat32(&writer, message.Y)
	writeFloat32(&writer, message.Z)
	return writer.Bytes(), nil
}

func DecodePlayerInput(payload []byte) (PlayerInput, error) {
	reader := bytes.NewReader(payload)
	objectID, err := readString(reader, MaxWireStringBytes)
	if err != nil {
		return PlayerInput{}, err
	}
	x, err := readFloat32(reader)
	if err != nil {
		return PlayerInput{}, err
	}
	y, err := readFloat32(reader)
	if err != nil {
		return PlayerInput{}, err
	}
	z, err := readFloat32(reader)
	if err != nil {
		return PlayerInput{}, err
	}
	if reader.Len() != 0 {
		return PlayerInput{}, fmt.Errorf("%w: trailing player input bytes", ErrMalformedWirePayload)
	}
	return PlayerInput{ObjectID: objectID, X: x, Y: y, Z: z}, nil
}

func EncodePing(message Ping) ([]byte, error) {
	writer := bytes.Buffer{}
	writeUint64(&writer, message.Nonce)
	return writer.Bytes(), nil
}

func DecodePing(payload []byte) (Ping, error) {
	reader := bytes.NewReader(payload)
	nonce, err := readUint64(reader)
	if err != nil {
		return Ping{}, err
	}
	if reader.Len() != 0 {
		return Ping{}, fmt.Errorf("%w: trailing ping bytes", ErrMalformedWirePayload)
	}
	return Ping{Nonce: nonce}, nil
}

func EncodePong(message Pong) ([]byte, error) {
	writer := bytes.Buffer{}
	writeUint64(&writer, message.Nonce)
	return writer.Bytes(), nil
}

func DecodePong(payload []byte) (Pong, error) {
	reader := bytes.NewReader(payload)
	nonce, err := readUint64(reader)
	if err != nil {
		return Pong{}, err
	}
	if reader.Len() != 0 {
		return Pong{}, fmt.Errorf("%w: trailing pong bytes", ErrMalformedWirePayload)
	}
	return Pong{Nonce: nonce}, nil
}

func EncodeChatMessage(message ChatMessage) ([]byte, error) {
	writer := bytes.Buffer{}
	if err := writeString(&writer, message.Text, MaxWireChatBytes); err != nil {
		return nil, err
	}
	return writer.Bytes(), nil
}

func DecodeChatMessage(payload []byte) (ChatMessage, error) {
	reader := bytes.NewReader(payload)
	text, err := readString(reader, MaxWireChatBytes)
	if err != nil {
		return ChatMessage{}, err
	}
	if reader.Len() != 0 {
		return ChatMessage{}, fmt.Errorf("%w: trailing chat bytes", ErrMalformedWirePayload)
	}
	return ChatMessage{Text: text}, nil
}

func EncodeWelcome(message Welcome) ([]byte, error) {
	writer := bytes.Buffer{}
	if err := writeString(&writer, message.ClientID, MaxWireStringBytes); err != nil {
		return nil, err
	}
	if err := writeString(&writer, message.AuthenticatedID, MaxWireStringBytes); err != nil {
		return nil, err
	}
	return writer.Bytes(), nil
}

func DecodeWelcome(payload []byte) (Welcome, error) {
	reader := bytes.NewReader(payload)
	clientID, err := readString(reader, MaxWireStringBytes)
	if err != nil {
		return Welcome{}, err
	}
	authenticatedID, err := readString(reader, MaxWireStringBytes)
	if err != nil {
		return Welcome{}, err
	}
	if reader.Len() != 0 {
		return Welcome{}, fmt.Errorf("%w: trailing welcome bytes", ErrMalformedWirePayload)
	}
	return Welcome{ClientID: clientID, AuthenticatedID: authenticatedID}, nil
}

func EncodeWorldSnapshot(message WorldSnapshot) ([]byte, error) {
	writer := bytes.Buffer{}
	if err := writeString(&writer, message.ClientID, MaxWireStringBytes); err != nil {
		return nil, err
	}
	writeUint64(&writer, message.Tick)
	writeUint64(&writer, message.LastSequence)
	if err := encodeSnapshotData(&writer, message.Objects); err != nil {
		return nil, err
	}
	if writer.Len() > MaxWireMessagePayloadSize {
		return nil, fmt.Errorf("%w: snapshot %d", ErrWireMessageTooLarge, writer.Len())
	}
	return writer.Bytes(), nil
}

func DecodeWorldSnapshot(payload []byte) (WorldSnapshot, error) {
	reader := bytes.NewReader(payload)
	clientID, err := readString(reader, MaxWireStringBytes)
	if err != nil {
		return WorldSnapshot{}, err
	}
	tick, err := readUint64(reader)
	if err != nil {
		return WorldSnapshot{}, err
	}
	lastSequence, err := readUint64(reader)
	if err != nil {
		return WorldSnapshot{}, err
	}
	objects, err := decodeSnapshotData(reader)
	if err != nil {
		return WorldSnapshot{}, err
	}
	if reader.Len() != 0 {
		return WorldSnapshot{}, fmt.Errorf("%w: trailing snapshot bytes", ErrMalformedWirePayload)
	}
	return WorldSnapshot{
		ClientID:     clientID,
		Tick:         tick,
		LastSequence: lastSequence,
		Objects:      objects,
	}, nil
}

func EncodeEntitySpawn(message EntitySpawn) ([]byte, error) {
	return encodeEntityPayload(message.ObjectType, message.ObjectID, message.Snapshot)
}

func DecodeEntitySpawn(payload []byte) (EntitySpawn, error) {
	objectType, objectID, snapshot, err := decodeEntityPayload(payload)
	return EntitySpawn{ObjectType: objectType, ObjectID: objectID, Snapshot: snapshot}, err
}

func EncodeEntityDespawn(message EntityDespawn) ([]byte, error) {
	writer := bytes.Buffer{}
	if err := writeString(&writer, message.ObjectType, MaxWireStringBytes); err != nil {
		return nil, err
	}
	if err := writeString(&writer, message.ObjectID, MaxWireStringBytes); err != nil {
		return nil, err
	}
	return writer.Bytes(), nil
}

func DecodeEntityDespawn(payload []byte) (EntityDespawn, error) {
	reader := bytes.NewReader(payload)
	objectType, err := readString(reader, MaxWireStringBytes)
	if err != nil {
		return EntityDespawn{}, err
	}
	objectID, err := readString(reader, MaxWireStringBytes)
	if err != nil {
		return EntityDespawn{}, err
	}
	if reader.Len() != 0 {
		return EntityDespawn{}, fmt.Errorf("%w: trailing despawn bytes", ErrMalformedWirePayload)
	}
	return EntityDespawn{ObjectType: objectType, ObjectID: objectID}, nil
}

func EncodeEntityUpdate(message EntityUpdate) ([]byte, error) {
	return encodeEntityPayload(message.ObjectType, message.ObjectID, message.Snapshot)
}

func DecodeEntityUpdate(payload []byte) (EntityUpdate, error) {
	objectType, objectID, snapshot, err := decodeEntityPayload(payload)
	return EntityUpdate{ObjectType: objectType, ObjectID: objectID, Snapshot: snapshot}, err
}

func EncodeErrorMessage(message ErrorMessage) ([]byte, error) {
	return encodeCodeMessage(message.Code, message.Message)
}

func DecodeErrorMessage(payload []byte) (ErrorMessage, error) {
	code, message, err := decodeCodeMessage(payload)
	return ErrorMessage{Code: code, Message: message}, err
}

func EncodeDisconnectMessage(message DisconnectMessage) ([]byte, error) {
	return encodeCodeMessage(message.Code, message.Message)
}

func DecodeDisconnectMessage(payload []byte) (DisconnectMessage, error) {
	code, message, err := decodeCodeMessage(payload)
	return DisconnectMessage{Code: code, Message: message}, err
}

func encodeEntityPayload(objectType, objectID string, snapshot any) ([]byte, error) {
	writer := bytes.Buffer{}
	if err := writeString(&writer, objectType, MaxWireStringBytes); err != nil {
		return nil, err
	}
	if err := writeString(&writer, objectID, MaxWireStringBytes); err != nil {
		return nil, err
	}
	if err := encodeWireValue(&writer, snapshot, 0); err != nil {
		return nil, err
	}
	return writer.Bytes(), nil
}

func decodeEntityPayload(payload []byte) (string, string, any, error) {
	reader := bytes.NewReader(payload)
	objectType, err := readString(reader, MaxWireStringBytes)
	if err != nil {
		return "", "", nil, err
	}
	objectID, err := readString(reader, MaxWireStringBytes)
	if err != nil {
		return "", "", nil, err
	}
	snapshot, err := decodeWireValue(reader, 0)
	if err != nil {
		return "", "", nil, err
	}
	if reader.Len() != 0 {
		return "", "", nil, fmt.Errorf("%w: trailing entity bytes", ErrMalformedWirePayload)
	}
	return objectType, objectID, snapshot, nil
}

func encodeCodeMessage(code uint16, message string) ([]byte, error) {
	writer := bytes.Buffer{}
	writeUint16(&writer, code)
	if err := writeString(&writer, message, MaxWireStringBytes); err != nil {
		return nil, err
	}
	return writer.Bytes(), nil
}

func decodeCodeMessage(payload []byte) (uint16, string, error) {
	reader := bytes.NewReader(payload)
	code, err := readUint16(reader)
	if err != nil {
		return 0, "", err
	}
	message, err := readString(reader, MaxWireStringBytes)
	if err != nil {
		return 0, "", err
	}
	if reader.Len() != 0 {
		return 0, "", fmt.Errorf("%w: trailing code message bytes", ErrMalformedWirePayload)
	}
	return code, message, nil
}

const (
	wireValueNull uint8 = iota
	wireValueBool
	wireValueInt64
	wireValueUint64
	wireValueFloat64
	wireValueString
	wireValueObject
	wireValueList
)

func encodeSnapshotData(writer *bytes.Buffer, snapshot SnapshotData) error {
	if len(snapshot) > 65535 {
		return fmt.Errorf("%w: too many object types", ErrWireMessageTooLarge)
	}
	objectTypes := make([]string, 0, len(snapshot))
	for objectType := range snapshot {
		objectTypes = append(objectTypes, objectType)
	}
	sort.Strings(objectTypes)

	writeUint16(writer, uint16(len(objectTypes)))
	for _, objectType := range objectTypes {
		if err := writeString(writer, objectType, MaxWireStringBytes); err != nil {
			return err
		}
		objectsByID := snapshot[objectType]
		if len(objectsByID) > 65535 {
			return fmt.Errorf("%w: too many objects", ErrWireMessageTooLarge)
		}
		writeUint16(writer, uint16(len(objectsByID)))

		objectIDs := make([]string, 0, len(objectsByID))
		for objectID := range objectsByID {
			objectIDs = append(objectIDs, objectID)
		}
		sort.Strings(objectIDs)

		for _, objectID := range objectIDs {
			if err := writeString(writer, objectID, MaxWireStringBytes); err != nil {
				return err
			}
			if err := encodeWireValue(writer, objectsByID[objectID], 0); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeSnapshotData(reader *bytes.Reader) (SnapshotData, error) {
	typeCount, err := readUint16(reader)
	if err != nil {
		return nil, err
	}
	snapshot := SnapshotData{}
	for typeIndex := 0; typeIndex < int(typeCount); typeIndex++ {
		objectType, err := readString(reader, MaxWireStringBytes)
		if err != nil {
			return nil, err
		}
		objectCount, err := readUint16(reader)
		if err != nil {
			return nil, err
		}
		snapshot[objectType] = map[string]any{}
		for objectIndex := 0; objectIndex < int(objectCount); objectIndex++ {
			objectID, err := readString(reader, MaxWireStringBytes)
			if err != nil {
				return nil, err
			}
			value, err := decodeWireValue(reader, 0)
			if err != nil {
				return nil, err
			}
			snapshot[objectType][objectID] = value
		}
	}
	return snapshot, nil
}

func encodeWireValue(writer *bytes.Buffer, value any, depth int) error {
	if depth > 16 {
		return fmt.Errorf("%w: nested value too deep", ErrUnsupportedWireSnapshotValue)
	}
	if value == nil {
		writer.WriteByte(wireValueNull)
		return nil
	}

	reflected := reflect.ValueOf(value)
	for reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface {
		if reflected.IsNil() {
			writer.WriteByte(wireValueNull)
			return nil
		}
		reflected = reflected.Elem()
	}

	switch reflected.Kind() {
	case reflect.Bool:
		writer.WriteByte(wireValueBool)
		if reflected.Bool() {
			writer.WriteByte(1)
		} else {
			writer.WriteByte(0)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		writer.WriteByte(wireValueInt64)
		writeUint64(writer, uint64(reflected.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		writer.WriteByte(wireValueUint64)
		writeUint64(writer, reflected.Uint())
	case reflect.Float32, reflect.Float64:
		writer.WriteByte(wireValueFloat64)
		writeFloat64(writer, reflected.Convert(reflect.TypeOf(float64(0))).Float())
	case reflect.String:
		writer.WriteByte(wireValueString)
		return writeString(writer, reflected.String(), MaxWireStringBytes)
	case reflect.Map:
		return encodeWireMap(writer, reflected, depth)
	case reflect.Struct:
		return encodeWireStruct(writer, reflected, depth)
	case reflect.Slice, reflect.Array:
		return encodeWireList(writer, reflected, depth)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedWireSnapshotValue, reflected.Kind())
	}

	return nil
}

func encodeWireMap(writer *bytes.Buffer, value reflect.Value, depth int) error {
	if value.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("%w: non-string map key", ErrUnsupportedWireSnapshotValue)
	}
	if value.Len() > 65535 {
		return fmt.Errorf("%w: map too large", ErrWireMessageTooLarge)
	}

	keys := make([]string, 0, value.Len())
	for _, key := range value.MapKeys() {
		keys = append(keys, key.String())
	}
	sort.Strings(keys)

	writer.WriteByte(wireValueObject)
	writeUint16(writer, uint16(len(keys)))
	for _, key := range keys {
		if err := writeString(writer, key, MaxWireStringBytes); err != nil {
			return err
		}
		if err := encodeWireValue(writer, value.MapIndex(reflect.ValueOf(key)).Interface(), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func encodeWireStruct(writer *bytes.Buffer, value reflect.Value, depth int) error {
	fields := exportedWireFields(value)
	if len(fields) > 65535 {
		return fmt.Errorf("%w: struct too large", ErrWireMessageTooLarge)
	}

	writer.WriteByte(wireValueObject)
	writeUint16(writer, uint16(len(fields)))
	for _, field := range fields {
		if err := writeString(writer, field.name, MaxWireStringBytes); err != nil {
			return err
		}
		if err := encodeWireValue(writer, field.value.Interface(), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func encodeWireList(writer *bytes.Buffer, value reflect.Value, depth int) error {
	if value.Len() > 65535 {
		return fmt.Errorf("%w: list too large", ErrWireMessageTooLarge)
	}

	writer.WriteByte(wireValueList)
	writeUint16(writer, uint16(value.Len()))
	for index := 0; index < value.Len(); index++ {
		if err := encodeWireValue(writer, value.Index(index).Interface(), depth+1); err != nil {
			return err
		}
	}
	return nil
}

type wireField struct {
	name  string
	value reflect.Value
}

func exportedWireFields(value reflect.Value) []wireField {
	valueType := value.Type()
	fields := make([]wireField, 0, value.NumField())
	for index := 0; index < value.NumField(); index++ {
		structField := valueType.Field(index)
		if structField.PkgPath != "" {
			continue
		}
		name, omitEmpty, ok := wireJSONName(structField)
		if !ok {
			continue
		}
		fieldValue := value.Field(index)
		if omitEmpty && fieldValue.IsZero() {
			continue
		}
		fields = append(fields, wireField{name: name, value: fieldValue})
	}
	sort.Slice(fields, func(left, right int) bool {
		return fields[left].name < fields[right].name
	})
	return fields
}

func wireJSONName(field reflect.StructField) (string, bool, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, false
	}
	if tag == "" {
		return field.Name, false, true
	}

	parts := strings.Split(tag, ",")
	name := parts[0]
	if name == "" {
		name = field.Name
	}
	omitEmpty := false
	for _, option := range parts[1:] {
		if option == "omitempty" {
			omitEmpty = true
			break
		}
	}
	return name, omitEmpty, true
}

func decodeWireValue(reader *bytes.Reader, depth int) (any, error) {
	if depth > 16 {
		return nil, fmt.Errorf("%w: nested value too deep", ErrMalformedWirePayload)
	}

	kind, err := reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("%w: missing value kind", ErrMalformedWirePayload)
	}
	switch kind {
	case wireValueNull:
		return nil, nil
	case wireValueBool:
		value, err := reader.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("%w: bool truncated", ErrMalformedWirePayload)
		}
		return value != 0, nil
	case wireValueInt64:
		value, err := readUint64(reader)
		return int64(value), err
	case wireValueUint64:
		value, err := readUint64(reader)
		return value, err
	case wireValueFloat64:
		return readFloat64(reader)
	case wireValueString:
		return readString(reader, MaxWireStringBytes)
	case wireValueObject:
		count, err := readUint16(reader)
		if err != nil {
			return nil, err
		}
		object := map[string]any{}
		for index := 0; index < int(count); index++ {
			key, err := readString(reader, MaxWireStringBytes)
			if err != nil {
				return nil, err
			}
			value, err := decodeWireValue(reader, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		return object, nil
	case wireValueList:
		count, err := readUint16(reader)
		if err != nil {
			return nil, err
		}
		values := make([]any, 0, count)
		for index := 0; index < int(count); index++ {
			value, err := decodeWireValue(reader, depth+1)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("%w: unknown value kind %d", ErrMalformedWirePayload, kind)
	}
}

func writeString(writer *bytes.Buffer, value string, maxBytes int) error {
	if len(value) > maxBytes || len(value) > 65535 {
		return fmt.Errorf("%w: %d", ErrWireStringTooLarge, len(value))
	}
	writeUint16(writer, uint16(len(value)))
	writer.WriteString(value)
	return nil
}

func readString(reader *bytes.Reader, maxBytes int) (string, error) {
	length, err := readUint16(reader)
	if err != nil {
		return "", err
	}
	if int(length) > maxBytes {
		return "", fmt.Errorf("%w: %d", ErrWireStringTooLarge, length)
	}
	if int(length) > reader.Len() {
		return "", fmt.Errorf("%w: string truncated", ErrMalformedWirePayload)
	}
	data := make([]byte, int(length))
	if _, err := reader.Read(data); err != nil {
		return "", err
	}
	return string(data), nil
}

func writeBytes(writer *bytes.Buffer, value []byte) {
	writeUint32(writer, uint32(len(value)))
	writer.Write(value)
}

func readBytes(reader *bytes.Reader, maxBytes int) ([]byte, error) {
	length, err := readUint32(reader)
	if err != nil {
		return nil, err
	}
	if int(length) > maxBytes {
		return nil, fmt.Errorf("%w: %d", ErrWireMessageTooLarge, length)
	}
	if int(length) > reader.Len() {
		return nil, fmt.Errorf("%w: bytes truncated", ErrMalformedWirePayload)
	}
	data := make([]byte, int(length))
	if _, err := reader.Read(data); err != nil {
		return nil, err
	}
	return data, nil
}

func writeUint16(writer *bytes.Buffer, value uint16) {
	var data [2]byte
	binary.BigEndian.PutUint16(data[:], value)
	writer.Write(data[:])
}

func readUint16(reader *bytes.Reader) (uint16, error) {
	var data [2]byte
	if _, err := reader.Read(data[:]); err != nil {
		return 0, fmt.Errorf("%w: uint16 truncated", ErrMalformedWirePayload)
	}
	return binary.BigEndian.Uint16(data[:]), nil
}

func writeUint32(writer *bytes.Buffer, value uint32) {
	var data [4]byte
	binary.BigEndian.PutUint32(data[:], value)
	writer.Write(data[:])
}

func readUint32(reader *bytes.Reader) (uint32, error) {
	var data [4]byte
	if _, err := reader.Read(data[:]); err != nil {
		return 0, fmt.Errorf("%w: uint32 truncated", ErrMalformedWirePayload)
	}
	return binary.BigEndian.Uint32(data[:]), nil
}

func writeUint64(writer *bytes.Buffer, value uint64) {
	var data [8]byte
	binary.BigEndian.PutUint64(data[:], value)
	writer.Write(data[:])
}

func readUint64(reader *bytes.Reader) (uint64, error) {
	var data [8]byte
	if _, err := reader.Read(data[:]); err != nil {
		return 0, fmt.Errorf("%w: uint64 truncated", ErrMalformedWirePayload)
	}
	return binary.BigEndian.Uint64(data[:]), nil
}

func writeFloat32(writer *bytes.Buffer, value float32) {
	writeUint32(writer, math.Float32bits(value))
}

func readFloat32(reader *bytes.Reader) (float32, error) {
	value, err := readUint32(reader)
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(value), nil
}

func writeFloat64(writer *bytes.Buffer, value float64) {
	writeUint64(writer, math.Float64bits(value))
}

func readFloat64(reader *bytes.Reader) (float64, error) {
	value, err := readUint64(reader)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(value), nil
}
