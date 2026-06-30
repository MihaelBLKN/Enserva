package network

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
)

const (
	// System messages are owned by the wire protocol itself.
	WireMessageSystemMin WireMessageType = 0x0000
	WireMessageSystemMax WireMessageType = 0x00ff
	// Engine messages are built-in Enserva gameplay/runtime adapters.
	WireMessageEngineMin WireMessageType = 0x0100
	WireMessageEngineMax WireMessageType = 0x0fff
	// Game messages are available to projects built on top of Enserva.
	WireMessageGameMin WireMessageType = 0x1000
	WireMessageGameMax WireMessageType = 0xffff
)

var (
	ErrWireMessageRegistered      = errors.New("wire message already registered")
	ErrWireMessageTypeOutOfRange  = errors.New("wire message type out of reserved range")
	ErrMissingWireMessageCodec    = errors.New("wire message is missing encoder or decoder")
	ErrMissingWireMessageName     = errors.New("wire message is missing name")
	ErrMissingWireMessageType     = errors.New("wire message is missing Go message type")
	ErrWireMessageValidation      = errors.New("wire message validation failed")
	ErrMissingWireMessageRegistry = errors.New("missing wire message registry")
)

var defaultWireMessageRegistry = NewDefaultWireMessageRegistry()

// DefaultWireMessages returns the process-wide registry used by package-level
// encode/decode helpers. Runtime instances keep their own registry so games can
// register custom messages without mutating global state.
func DefaultWireMessages() *WireMessageRegistry {
	return defaultWireMessageRegistry
}

// WireMessageDirection documents whether a definition is expected from clients,
// servers, or both. The registry keeps this as metadata. Transports decide how
// strict to be when dispatching.
type WireMessageDirection uint8

const (
	WireDirectionBoth WireMessageDirection = iota
	WireDirectionClientToServer
	WireDirectionServerToClient
)

// WireMessageEncoder converts a typed message into its payload bytes.
type WireMessageEncoder func(any) ([]byte, error)

// WireMessageDecoder converts payload bytes into a typed message.
type WireMessageDecoder func([]byte) (any, error)

// WireMessageValidator checks semantic constraints after decode or before encode.
type WireMessageValidator func(any) error

// WireMessageHandler dispatches decoded messages from a transport into game or
// protocol logic. Custom game messages can use ctx.Runtime to route into systems.
type WireMessageHandler func(WireMessageContext) error

// WireMessageContext is passed to registered message handlers.
type WireMessageContext struct {
	Transport  string
	ClientID   string
	Sequence   uint64
	Ack        uint64
	AckBits    uint64
	ReceivedAt time.Time
	MessageID  WireMessageType
	Message    any
	Runtime    *Runtime
	Response   ResponseWriter
}

// WireMessageDefinition describes one stable wire message type.
type WireMessageDefinition struct {
	ID          WireMessageType
	Name        string
	Direction   WireMessageDirection
	MessageType reflect.Type
	Encode      WireMessageEncoder
	Decode      WireMessageDecoder
	Validate    WireMessageValidator
	Handler     WireMessageHandler
}

// WireMessageRegistry stores wire schemas by numeric ID and Go message type.
type WireMessageRegistry struct {
	byID   map[WireMessageType]WireMessageDefinition
	byType map[reflect.Type]WireMessageDefinition
	mu     sync.RWMutex
}

// NewWireMessageRegistry creates an empty registry.
func NewWireMessageRegistry() *WireMessageRegistry {
	return &WireMessageRegistry{
		byID:   map[WireMessageType]WireMessageDefinition{},
		byType: map[reflect.Type]WireMessageDefinition{},
	}
}

// NewDefaultWireMessageRegistry creates a registry with Enserva's built-in
// protocol and compatibility messages registered.
func NewDefaultWireMessageRegistry() *WireMessageRegistry {
	registry := NewWireMessageRegistry()
	mustRegisterWireMessage(registry, WireMessageDefinition{
		ID:          WireMessageClientHello,
		Name:        "protocol.hello",
		Direction:   WireDirectionClientToServer,
		MessageType: reflect.TypeOf(ClientHello{}),
		Encode:      encodeClientHelloAny,
		Decode:      decodeClientHelloAny,
		Validate:    validateClientHello,
	})
	mustRegisterWireMessage(registry, WireMessageDefinition{
		ID:          WireMessageWelcome,
		Name:        "protocol.welcome",
		Direction:   WireDirectionServerToClient,
		MessageType: reflect.TypeOf(Welcome{}),
		Encode:      encodeWelcomeAny,
		Decode:      decodeWelcomeAny,
	})
	mustRegisterWireMessage(registry, WireMessageDefinition{
		ID:          WireMessagePing,
		Name:        "protocol.ping",
		Direction:   WireDirectionClientToServer,
		MessageType: reflect.TypeOf(Ping{}),
		Encode:      encodePingAny,
		Decode:      decodePingAny,
		Handler:     handleWirePing,
	})
	mustRegisterWireMessage(registry, WireMessageDefinition{
		ID:          WireMessagePong,
		Name:        "protocol.pong",
		Direction:   WireDirectionServerToClient,
		MessageType: reflect.TypeOf(Pong{}),
		Encode:      encodePongAny,
		Decode:      decodePongAny,
	})
	mustRegisterWireMessage(registry, WireMessageDefinition{
		ID:          WireMessageError,
		Name:        "protocol.error",
		Direction:   WireDirectionServerToClient,
		MessageType: reflect.TypeOf(ErrorMessage{}),
		Encode:      encodeErrorMessageAny,
		Decode:      decodeErrorMessageAny,
		Validate:    validateCodeMessage,
	})
	mustRegisterWireMessage(registry, WireMessageDefinition{
		ID:          WireMessageDisconnect,
		Name:        "protocol.disconnect",
		Direction:   WireDirectionServerToClient,
		MessageType: reflect.TypeOf(DisconnectMessage{}),
		Encode:      encodeDisconnectMessageAny,
		Decode:      decodeDisconnectMessageAny,
		Validate:    validateCodeMessage,
	})
	mustRegisterWireMessage(registry, WireMessageDefinition{
		ID:          WireMessageObjectRequest,
		Name:        "engine.object_request",
		Direction:   WireDirectionClientToServer,
		MessageType: reflect.TypeOf(ObjectRequest{}),
		Encode:      encodeObjectRequestAny,
		Decode:      decodeObjectRequestAny,
		Validate:    validateObjectRequest,
	})
	mustRegisterWireMessage(registry, WireMessageDefinition{
		ID:          WireMessagePlayerInput,
		Name:        "engine.player_input",
		Direction:   WireDirectionClientToServer,
		MessageType: reflect.TypeOf(PlayerInput{}),
		Encode:      encodePlayerInputAny,
		Decode:      decodePlayerInputAny,
		Validate:    validatePlayerInput,
	})
	mustRegisterWireMessage(registry, WireMessageDefinition{
		ID:          WireMessageWorldSnapshot,
		Name:        "engine.world_snapshot",
		Direction:   WireDirectionServerToClient,
		MessageType: reflect.TypeOf(WorldSnapshot{}),
		Encode:      encodeWorldSnapshotAny,
		Decode:      decodeWorldSnapshotAny,
	})
	mustRegisterWireMessage(registry, WireMessageDefinition{
		ID:          WireMessageEntitySpawn,
		Name:        "engine.entity_spawn",
		Direction:   WireDirectionServerToClient,
		MessageType: reflect.TypeOf(EntitySpawn{}),
		Encode:      encodeEntitySpawnAny,
		Decode:      decodeEntitySpawnAny,
	})
	mustRegisterWireMessage(registry, WireMessageDefinition{
		ID:          WireMessageEntityDespawn,
		Name:        "engine.entity_despawn",
		Direction:   WireDirectionServerToClient,
		MessageType: reflect.TypeOf(EntityDespawn{}),
		Encode:      encodeEntityDespawnAny,
		Decode:      decodeEntityDespawnAny,
	})
	mustRegisterWireMessage(registry, WireMessageDefinition{
		ID:          WireMessageEntityUpdate,
		Name:        "engine.entity_update",
		Direction:   WireDirectionServerToClient,
		MessageType: reflect.TypeOf(EntityUpdate{}),
		Encode:      encodeEntityUpdateAny,
		Decode:      decodeEntityUpdateAny,
	})
	return registry
}

func mustRegisterWireMessage(registry *WireMessageRegistry, definition WireMessageDefinition) {
	if err := registry.Register(definition); err != nil {
		panic(err)
	}
}

// Register adds one message definition. Game projects should use IDs in
// 0x1000-0xffff for custom messages.
func (registry *WireMessageRegistry) Register(definition WireMessageDefinition) error {
	if registry == nil {
		return ErrMissingWireMessageRegistry
	}
	if definition.Name == "" {
		return ErrMissingWireMessageName
	}
	if definition.MessageType == nil {
		return ErrMissingWireMessageType
	}
	if definition.MessageType.Kind() == reflect.Pointer {
		definition.MessageType = definition.MessageType.Elem()
	}
	if definition.Encode == nil || definition.Decode == nil {
		return ErrMissingWireMessageCodec
	}
	if !wireMessageIDInKnownRange(definition.ID) {
		return fmt.Errorf("%w: 0x%04x", ErrWireMessageTypeOutOfRange, definition.ID)
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()

	if _, ok := registry.byID[definition.ID]; ok {
		return fmt.Errorf("%w: id 0x%04x", ErrWireMessageRegistered, definition.ID)
	}
	if _, ok := registry.byType[definition.MessageType]; ok {
		return fmt.Errorf("%w: type %s", ErrWireMessageRegistered, definition.MessageType)
	}

	registry.byID[definition.ID] = definition
	registry.byType[definition.MessageType] = definition
	return nil
}

func wireMessageIDInKnownRange(id WireMessageType) bool {
	return (id >= WireMessageSystemMin && id <= WireMessageSystemMax) ||
		(id >= WireMessageEngineMin && id <= WireMessageEngineMax) ||
		(id >= WireMessageGameMin && id <= WireMessageGameMax)
}

// Definition returns the registered definition for id.
func (registry *WireMessageRegistry) Definition(id WireMessageType) (WireMessageDefinition, bool) {
	if registry == nil {
		return WireMessageDefinition{}, false
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	definition, ok := registry.byID[id]
	return definition, ok
}

// EncodeMessage encodes a typed message using its registered definition.
func (registry *WireMessageRegistry) EncodeMessage(message any) (WireMessage, error) {
	if registry == nil {
		return WireMessage{}, ErrMissingWireMessageRegistry
	}
	messageType := reflect.TypeOf(message)
	if messageType == nil {
		return WireMessage{}, fmt.Errorf("%w: <nil>", ErrUnsupportedWireMessage)
	}
	if messageType.Kind() == reflect.Pointer {
		messageType = messageType.Elem()
	}

	registry.mu.RLock()
	definition, ok := registry.byType[messageType]
	registry.mu.RUnlock()
	if !ok {
		return WireMessage{}, fmt.Errorf("%w: %s", ErrUnsupportedWireMessage, messageType)
	}

	if err := validateWireMessage(definition, message); err != nil {
		return WireMessage{}, err
	}
	payload, err := definition.Encode(message)
	if err != nil {
		return WireMessage{}, err
	}
	return WireMessage{Type: definition.ID, Payload: payload}, nil
}

// DecodeMessage decodes a framed message using its registered definition.
func (registry *WireMessageRegistry) DecodeMessage(message WireMessage) (any, error) {
	if registry == nil {
		return nil, ErrMissingWireMessageRegistry
	}
	definition, ok := registry.Definition(message.Type)
	if !ok {
		return UnknownWireMessage{Type: message.Type, Payload: append([]byte(nil), message.Payload...)}, nil
	}

	decoded, err := definition.Decode(message.Payload)
	if err != nil {
		return nil, err
	}
	if err := validateWireMessage(definition, decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

// Dispatch invokes the registered handler for a decoded message when one exists.
func (registry *WireMessageRegistry) Dispatch(ctx WireMessageContext) (bool, error) {
	if registry == nil {
		return false, ErrMissingWireMessageRegistry
	}
	definition, ok := registry.Definition(ctx.MessageID)
	if !ok || definition.Handler == nil {
		return false, nil
	}
	return true, definition.Handler(ctx)
}

func validateWireMessage(definition WireMessageDefinition, message any) error {
	if definition.Validate == nil {
		return nil
	}
	if err := definition.Validate(message); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrWireMessageValidation, definition.Name, err)
	}
	return nil
}

func handleWirePing(ctx WireMessageContext) error {
	ping, ok := ctx.Message.(Ping)
	if !ok || ctx.Response == nil {
		return nil
	}
	return ctx.Response.Respond(Pong{Nonce: ping.Nonce})
}

func validateClientHello(message any) error {
	hello, ok := message.(ClientHello)
	if !ok {
		if pointer, ok := message.(*ClientHello); ok {
			hello = *pointer
		} else {
			return fmt.Errorf("expected ClientHello")
		}
	}
	if len(hello.ClientName) > MaxWireStringBytes || len(hello.Token) > MaxWireStringBytes {
		return ErrWireStringTooLarge
	}
	return nil
}

func validateObjectRequest(message any) error {
	request, ok := message.(ObjectRequest)
	if !ok {
		if pointer, ok := message.(*ObjectRequest); ok {
			request = *pointer
		} else {
			return fmt.Errorf("expected ObjectRequest")
		}
	}
	if request.ObjectType == "" {
		return ErrMissingObjectType
	}
	if request.ObjectID == "" {
		return ErrMissingObjectID
	}
	if len(request.Data) > MaxWireMessagePayloadSize {
		return ErrWireMessageTooLarge
	}
	return nil
}

func validatePlayerInput(message any) error {
	input, ok := message.(PlayerInput)
	if !ok {
		if pointer, ok := message.(*PlayerInput); ok {
			input = *pointer
		} else {
			return fmt.Errorf("expected PlayerInput")
		}
	}
	if input.ObjectID == "" {
		return ErrMissingObjectID
	}
	return nil
}

func validateCodeMessage(message any) error {
	switch value := message.(type) {
	case ErrorMessage:
		if len(value.Message) > MaxWireStringBytes {
			return ErrWireStringTooLarge
		}
	case DisconnectMessage:
		if len(value.Message) > MaxWireStringBytes {
			return ErrWireStringTooLarge
		}
	}
	return nil
}

func encodeClientHelloAny(message any) ([]byte, error) {
	value, ok := message.(ClientHello)
	if !ok {
		value = *(message.(*ClientHello))
	}
	return EncodeClientHello(value)
}

func decodeClientHelloAny(payload []byte) (any, error) { return DecodeClientHello(payload) }

func encodeObjectRequestAny(message any) ([]byte, error) {
	value, ok := message.(ObjectRequest)
	if !ok {
		value = *(message.(*ObjectRequest))
	}
	return EncodeObjectRequest(value)
}

func decodeObjectRequestAny(payload []byte) (any, error) { return DecodeObjectRequest(payload) }

func encodePlayerInputAny(message any) ([]byte, error) {
	value, ok := message.(PlayerInput)
	if !ok {
		value = *(message.(*PlayerInput))
	}
	return EncodePlayerInput(value)
}

func decodePlayerInputAny(payload []byte) (any, error) { return DecodePlayerInput(payload) }

func encodePingAny(message any) ([]byte, error) {
	value, ok := message.(Ping)
	if !ok {
		value = *(message.(*Ping))
	}
	return EncodePing(value)
}

func decodePingAny(payload []byte) (any, error) { return DecodePing(payload) }

func encodePongAny(message any) ([]byte, error) {
	value, ok := message.(Pong)
	if !ok {
		value = *(message.(*Pong))
	}
	return EncodePong(value)
}

func decodePongAny(payload []byte) (any, error) { return DecodePong(payload) }

func encodeWelcomeAny(message any) ([]byte, error) {
	value, ok := message.(Welcome)
	if !ok {
		value = *(message.(*Welcome))
	}
	return EncodeWelcome(value)
}

func decodeWelcomeAny(payload []byte) (any, error) { return DecodeWelcome(payload) }

func encodeWorldSnapshotAny(message any) ([]byte, error) {
	value, ok := message.(WorldSnapshot)
	if !ok {
		value = *(message.(*WorldSnapshot))
	}
	return EncodeWorldSnapshot(value)
}

func decodeWorldSnapshotAny(payload []byte) (any, error) { return DecodeWorldSnapshot(payload) }

func encodeEntitySpawnAny(message any) ([]byte, error) {
	value, ok := message.(EntitySpawn)
	if !ok {
		value = *(message.(*EntitySpawn))
	}
	return EncodeEntitySpawn(value)
}

func decodeEntitySpawnAny(payload []byte) (any, error) { return DecodeEntitySpawn(payload) }

func encodeEntityDespawnAny(message any) ([]byte, error) {
	value, ok := message.(EntityDespawn)
	if !ok {
		value = *(message.(*EntityDespawn))
	}
	return EncodeEntityDespawn(value)
}

func decodeEntityDespawnAny(payload []byte) (any, error) { return DecodeEntityDespawn(payload) }

func encodeEntityUpdateAny(message any) ([]byte, error) {
	value, ok := message.(EntityUpdate)
	if !ok {
		value = *(message.(*EntityUpdate))
	}
	return EncodeEntityUpdate(value)
}

func decodeEntityUpdateAny(payload []byte) (any, error) { return DecodeEntityUpdate(payload) }

func encodeErrorMessageAny(message any) ([]byte, error) {
	value, ok := message.(ErrorMessage)
	if !ok {
		value = *(message.(*ErrorMessage))
	}
	return EncodeErrorMessage(value)
}

func decodeErrorMessageAny(payload []byte) (any, error) { return DecodeErrorMessage(payload) }

func encodeDisconnectMessageAny(message any) ([]byte, error) {
	value, ok := message.(DisconnectMessage)
	if !ok {
		value = *(message.(*DisconnectMessage))
	}
	return EncodeDisconnectMessage(value)
}

func decodeDisconnectMessageAny(payload []byte) (any, error) { return DecodeDisconnectMessage(payload) }
