package network

import (
	"bytes"
	"errors"
	"fmt"
	"time"
)

const (
	defaultReliableRetryInterval = 100 * time.Millisecond
	defaultReliableMaxAttempts   = 5
	defaultReliableQueueLimit    = 64
)

var (
	ErrInvalidDeliveryClass = errors.New("invalid delivery class")
	ErrReliableMessageID    = errors.New("invalid reliable message id")
	ErrReliableQueueFull    = errors.New("reliable queue full")
)

// OutboundPriority orders outbound traffic when a client is over its byte
// budget. Higher values are sent before lower values.
type OutboundPriority int

const (
	OutboundPriorityLow       OutboundPriority = -100
	OutboundPriorityNormal    OutboundPriority = 0
	OutboundPriorityHigh      OutboundPriority = 100
	OutboundPriorityEssential OutboundPriority = 1000
)

// DeliveryClass describes how a wire message should be delivered over UDP.
type DeliveryClass uint8

const (
	DeliveryUnreliable DeliveryClass = iota
	DeliveryReliableOrdered
	DeliveryReliableUnordered
)

// WireDelivery wraps a response or outbound message with explicit delivery
// metadata. Unwrapped messages remain unreliable.
type WireDelivery struct {
	Message any
	Class   DeliveryClass
}

// WirePriority wraps a response or outbound message with generic priority
// metadata. Unwrapped messages use Config.DefaultSnapshotPriority for snapshot
// objects and normal protocol defaults for responses.
type WirePriority struct {
	Message  any
	Priority OutboundPriority
}

// Prioritize marks message with an explicit outbound priority.
func Prioritize(message any, priority OutboundPriority) WirePriority {
	return WirePriority{Message: message, Priority: priority}
}

// PrioritizeLow marks message as lower priority than default traffic.
func PrioritizeLow(message any) WirePriority {
	return Prioritize(message, OutboundPriorityLow)
}

// PrioritizeHigh marks message as higher priority than default traffic.
func PrioritizeHigh(message any) WirePriority {
	return Prioritize(message, OutboundPriorityHigh)
}

// PrioritizeEssential marks message as essential protocol traffic.
func PrioritizeEssential(message any) WirePriority {
	return Prioritize(message, OutboundPriorityEssential)
}

// Deliver marks message with an explicit delivery class.
func Deliver(message any, class DeliveryClass) WireDelivery {
	return WireDelivery{Message: message, Class: class}
}

// DeliverReliableOrdered marks message for reliable ordered delivery.
func DeliverReliableOrdered(message any) WireDelivery {
	return Deliver(message, DeliveryReliableOrdered)
}

// DeliverReliableUnordered marks message for reliable unordered delivery.
func DeliverReliableUnordered(message any) WireDelivery {
	return Deliver(message, DeliveryReliableUnordered)
}

// DeliverUnreliable marks message for the default unreliable delivery class.
func DeliverUnreliable(message any) WireDelivery {
	return Deliver(message, DeliveryUnreliable)
}

func (class DeliveryClass) reliable() bool {
	return class == DeliveryReliableOrdered || class == DeliveryReliableUnordered
}

func validateDeliveryClass(class DeliveryClass) error {
	switch class {
	case DeliveryUnreliable, DeliveryReliableOrdered, DeliveryReliableUnordered:
		return nil
	default:
		return fmt.Errorf("%w: %d", ErrInvalidDeliveryClass, class)
	}
}

func encodeReliableEnvelope(message WireMessage) ([]byte, error) {
	if err := validateDeliveryClass(message.Delivery); err != nil {
		return nil, err
	}
	if !message.Delivery.reliable() {
		return nil, fmt.Errorf("%w: %d", ErrInvalidDeliveryClass, message.Delivery)
	}
	if message.DeliveryID == 0 {
		return nil, ErrReliableMessageID
	}
	if len(message.Payload) > MaxWireMessagePayloadSize {
		return nil, fmt.Errorf("%w: %d", ErrWireMessageTooLarge, len(message.Payload))
	}

	writer := bytes.Buffer{}
	writer.WriteByte(byte(message.Delivery))
	writeUint64(&writer, message.DeliveryID)
	writeUint16(&writer, uint16(message.Type))
	writeUint32(&writer, uint32(len(message.Payload)))
	writer.Write(message.Payload)
	if writer.Len() > MaxWireMessagePayloadSize {
		return nil, fmt.Errorf("%w: %d", ErrWireMessageTooLarge, writer.Len())
	}
	return writer.Bytes(), nil
}

func decodeReliableEnvelope(payload []byte) (WireMessage, error) {
	reader := bytes.NewReader(payload)
	deliveryByte, err := reader.ReadByte()
	if err != nil {
		return WireMessage{}, fmt.Errorf("%w: reliable delivery missing", ErrMalformedWirePayload)
	}
	delivery := DeliveryClass(deliveryByte)
	if err := validateDeliveryClass(delivery); err != nil {
		return WireMessage{}, err
	}
	if !delivery.reliable() {
		return WireMessage{}, fmt.Errorf("%w: %d", ErrInvalidDeliveryClass, delivery)
	}
	deliveryID, err := readUint64(reader)
	if err != nil {
		return WireMessage{}, err
	}
	if deliveryID == 0 {
		return WireMessage{}, ErrReliableMessageID
	}
	messageType, err := readUint16(reader)
	if err != nil {
		return WireMessage{}, err
	}
	messageLength, err := readUint32(reader)
	if err != nil {
		return WireMessage{}, err
	}
	if messageLength > MaxWireMessagePayloadSize {
		return WireMessage{}, fmt.Errorf("%w: %d", ErrWireMessageTooLarge, messageLength)
	}
	if int(messageLength) > reader.Len() {
		return WireMessage{}, fmt.Errorf("%w: reliable payload truncated", ErrMalformedWirePayload)
	}
	messagePayload := make([]byte, int(messageLength))
	if _, err := reader.Read(messagePayload); err != nil {
		return WireMessage{}, err
	}
	if reader.Len() != 0 {
		return WireMessage{}, fmt.Errorf("%w: trailing reliable envelope bytes", ErrMalformedWirePayload)
	}

	return WireMessage{
		Type:       WireMessageType(messageType),
		Payload:    messagePayload,
		Delivery:   delivery,
		DeliveryID: deliveryID,
	}, nil
}

func packetSequenceAcked(ack, ackBits, sequence uint64) bool {
	if sequence == 0 || ack == 0 {
		return false
	}
	if sequence == ack {
		return true
	}
	if sequence > ack {
		return false
	}
	distance := ack - sequence
	if distance == 0 || distance > 64 {
		return false
	}
	return ackBits&(uint64(1)<<(distance-1)) != 0
}
