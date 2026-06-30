package network

import (
	"encoding/json"
	"time"
)

const (
	defaultTickRate      = 128
	defaultSnapshotRate  = 20
	defaultClientTimeout = 5 * time.Second
	defaultDebugAddress  = ":9100"
)

type Config struct {
	UDPAddress    string
	TickRate      int
	SnapshotRate  int
	ClientTimeout time.Duration
	DebugEnabled  bool
	DebugAddress  string
}

// DefaultConfig returns the default server configuration.
func DefaultConfig() Config {
	return Config{
		UDPAddress:    ":9000",
		TickRate:      defaultTickRate,
		SnapshotRate:  defaultSnapshotRate,
		ClientTimeout: defaultClientTimeout,
		DebugAddress:  defaultDebugAddress,
	}
}

// Normalized fills missing configuration fields and clamps invalid rates.
func (config Config) Normalized() Config {
	defaults := DefaultConfig()

	if config.UDPAddress == "" {
		config.UDPAddress = defaults.UDPAddress
	}
	if config.TickRate <= 0 {
		config.TickRate = defaults.TickRate
	}
	if config.SnapshotRate <= 0 {
		config.SnapshotRate = defaults.SnapshotRate
	}
	if config.SnapshotRate > config.TickRate {
		config.SnapshotRate = config.TickRate
	}
	if config.ClientTimeout <= 0 {
		config.ClientTimeout = defaults.ClientTimeout
	}
	if config.DebugAddress == "" {
		config.DebugAddress = defaults.DebugAddress
	}

	return config
}

// TickInterval returns the duration between simulation ticks.
func (config Config) TickInterval() time.Duration {
	config = config.Normalized()

	return time.Second / time.Duration(config.TickRate)
}

// SnapshotEvery returns the tick interval between outbound snapshots.
func (config Config) SnapshotEvery() uint64 {
	config = config.Normalized()

	every := config.TickRate / config.SnapshotRate
	if every <= 0 {
		return 1
	}

	return uint64(every)
}

// Object is the minimal interface for values managed by the runtime.
type Object interface {
	ObjectType() string
	ObjectID() string
	Snapshot() any
}

// InitHandler runs after an object is registered with a runtime.
type InitHandler interface {
	OnInit(InitContext)
}

// TickHandler runs on every simulation tick.
type TickHandler interface {
	OnTick(TickContext)
}

// FullTickHandler runs once per configured tick-rate window.
type FullTickHandler interface {
	OnFullTick(TickContext)
}

// RequestHandler handles client requests routed to an object.
type RequestHandler interface {
	OnRequest(RequestContext) error
}

// AuthenticationHandler validates authentication requests and returns the authenticated client id.
type AuthenticationHandler interface {
	OnAuthenticationAttempt(AuthenticationContext) (string, error)
}

// SnapshotVisibility controls whether an object is included in snapshots.
type SnapshotVisibility interface {
	SnapshotVisible() bool
}

// ObjectFactory creates runtime objects from server-side requests.
type ObjectFactory interface {
	CreateObject(RequestContext) (Object, error)
}

// ObjectFactoryFunc adapts a function into an ObjectFactory.
type ObjectFactoryFunc func(RequestContext) (Object, error)

// CreateObject calls factory with ctx.
func (factory ObjectFactoryFunc) CreateObject(ctx RequestContext) (Object, error) {
	return factory(ctx)
}

// RequestMessage is the compatibility request envelope used by legacy JSON
// datagrams and by wire messages that adapt into object requests.
type RequestMessage struct {
	Type       string          `json:"type,omitempty"`
	Sequence   uint64          `json:"seq,omitempty"`
	ObjectType string          `json:"objectType"`
	ObjectID   string          `json:"objectId"`
	Action     string          `json:"action,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
}

// SnapshotData groups object snapshots by object type and id.
type SnapshotData map[string]map[string]any

// SnapshotMessage is sent by transports to publish the current runtime state.
type SnapshotMessage struct {
	Type         string       `json:"type"`
	ClientID     string       `json:"clientId,omitempty"`
	Tick         uint64       `json:"tick"`
	LastSequence uint64       `json:"lastSeq,omitempty"`
	Objects      SnapshotData `json:"objects"`
}

// ResponseMessage is the standard request response envelope.
type ResponseMessage struct {
	Type     string `json:"type"`
	Sequence uint64 `json:"seq,omitempty"`
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	Data     any    `json:"data,omitempty"`
}

// ResponseWriter sends an immediate response to a request.
type ResponseWriter interface {
	Respond(message any) error
}

// ResponseWriterFunc adapts a function into a ResponseWriter.
type ResponseWriterFunc func(message any) error

// Respond sends message through writer.
func (writer ResponseWriterFunc) Respond(message any) error {
	if writer == nil {
		return ErrResponsesUnsupported
	}

	return writer(message)
}

// AuthenticationResponse reports a successful authentication attempt.
type AuthenticationResponse struct {
	Type            string `json:"type"`
	Sequence        uint64 `json:"seq,omitempty"`
	OK              bool   `json:"ok"`
	ClientID        string `json:"clientId"`
	AuthenticatedID string `json:"authenticatedId"`
}

// InitContext describes the object being initialized.
type InitContext struct {
	object     Object
	objectType string
	objectID   string
	runtime    *Runtime
}

// Object returns the object being initialized.
func (ctx InitContext) Object() Object {
	return ctx.object
}

// ObjectType returns the initialized object's type.
func (ctx InitContext) ObjectType() string {
	return ctx.objectType
}

// ObjectID returns the initialized object's id.
func (ctx InitContext) ObjectID() string {
	return ctx.objectID
}

// Runtime returns the runtime that owns the initialized object.
func (ctx InitContext) Runtime() *Runtime {
	return ctx.runtime
}

// TickContext describes the current simulation tick.
type TickContext struct {
	Tick         uint64
	Delta        time.Duration
	DeltaSeconds float64
	Runtime      *Runtime
	Features     *Features
}

// RequestContext describes a client request routed through the runtime.
type RequestContext struct {
	Transport  string
	ClientID   string
	Tick       uint64
	ReceivedAt time.Time
	Request    RequestMessage
	Payload    any
	Object     Object
	Runtime    *Runtime
	Features   *Features
	Response   ResponseWriter
}

// Decode copies the protocol-decoded payload, or unmarshals legacy JSON data, into target.
func (ctx RequestContext) Decode(target any) error {
	if ctx.Payload != nil {
		return decodePayload(ctx.Payload, target)
	}
	if len(ctx.Request.Data) == 0 {
		return nil
	}

	return json.Unmarshal(ctx.Request.Data, target)
}

// Respond sends an immediate response through the request transport.
func (ctx RequestContext) Respond(message any) error {
	if ctx.Response == nil {
		return ErrResponsesUnsupported
	}

	return ctx.Response.Respond(message)
}

// RequestSceneSwitch asks the runtime to move the current object to targetScene.
func (ctx RequestContext) RequestSceneSwitch(targetScene SceneID) (SceneSwitchDecision, error) {
	if ctx.Runtime == nil {
		return SceneSwitchDecision{}, ErrMissingSceneRuntime
	}

	return ctx.Runtime.requestSceneSwitch(SceneSwitchContext{
		Transport:   ctx.Transport,
		ClientID:    ctx.ClientID,
		Tick:        ctx.Tick,
		ReceivedAt:  ctx.ReceivedAt,
		Request:     ctx.Request,
		Payload:     ctx.Payload,
		Object:      ctx.Object,
		ObjectType:  ctx.Request.ObjectType,
		ObjectID:    ctx.Request.ObjectID,
		TargetScene: targetScene,
		Runtime:     ctx.Runtime,
		Features:    ctx.Features,
		Response:    ctx.Response,
	})
}

// SceneSwitchHandler validates scene switch requests for an object.
type SceneSwitchHandler interface {
	OnSceneSwitchRequest(SceneSwitchContext) (SceneSwitchDecision, error)
}

// SceneSwitchContext describes a server-owned scene switch request.
type SceneSwitchContext struct {
	Transport    string
	ClientID     string
	Tick         uint64
	ReceivedAt   time.Time
	Request      RequestMessage
	Payload      any
	Object       Object
	ObjectType   string
	ObjectID     string
	CurrentScene SceneID
	TargetScene  SceneID
	Runtime      *Runtime
	Features     *Features
	Response     ResponseWriter
}

// Decode copies the protocol-decoded payload, or unmarshals legacy JSON data, into target.
func (ctx SceneSwitchContext) Decode(target any) error {
	if ctx.Payload != nil {
		return decodePayload(ctx.Payload, target)
	}
	if len(ctx.Request.Data) == 0 {
		return nil
	}

	return json.Unmarshal(ctx.Request.Data, target)
}

// Respond sends an immediate scene switch response through the request transport.
func (ctx SceneSwitchContext) Respond(message any) error {
	if ctx.Response == nil {
		return ErrResponsesUnsupported
	}

	return ctx.Response.Respond(message)
}

// SceneSwitchDecision describes whether a scene switch was accepted.
type SceneSwitchDecision struct {
	Allowed            bool    `json:"allowed"`
	Scene              SceneID `json:"scene,omitempty"`
	Reason             string  `json:"reason,omitempty"`
	ClearClientObjects bool    `json:"clearClientObjects,omitempty"`
	Data               any     `json:"data,omitempty"`
}

// SceneSwitchAllowed accepts a switch to the requested scene.
func SceneSwitchAllowed() SceneSwitchDecision {
	return SceneSwitchDecision{Allowed: true, ClearClientObjects: true}
}

// SceneSwitchAllowedTo accepts a switch and redirects it to sceneID.
func SceneSwitchAllowedTo(sceneID SceneID) SceneSwitchDecision {
	decision := SceneSwitchAllowed()
	decision.Scene = sceneID
	return decision
}

// SceneSwitchDenied rejects a switch with a reason code or message.
func SceneSwitchDenied(reason string) SceneSwitchDecision {
	return SceneSwitchDecision{Allowed: false, Reason: reason}
}

// SceneSwitchRequest is a standard payload for client scene switch requests.
type SceneSwitchRequest struct {
	TargetScene SceneID `json:"targetScene"`
}

// SceneSwitchResponse is the standard immediate response for scene switch requests.
type SceneSwitchResponse struct {
	Type               string  `json:"type"`
	Sequence           uint64  `json:"seq,omitempty"`
	OK                 bool    `json:"ok"`
	Scene              SceneID `json:"scene,omitempty"`
	PreviousScene      SceneID `json:"previousScene,omitempty"`
	Reason             string  `json:"reason,omitempty"`
	ClearClientObjects bool    `json:"clearClientObjects,omitempty"`
	Data               any     `json:"data,omitempty"`
}

// AuthenticationContext describes an authentication request.
type AuthenticationContext struct {
	Transport    string
	ConnectionID string
	ClientID     string
	Tick         uint64
	ReceivedAt   time.Time
	Request      RequestMessage
	Payload      any
	Runtime      *Runtime
	Features     *Features
}

// Decode copies the protocol-decoded payload, or unmarshals legacy JSON data, into target.
func (ctx AuthenticationContext) Decode(target any) error {
	if ctx.Payload != nil {
		return decodePayload(ctx.Payload, target)
	}
	if len(ctx.Request.Data) == 0 {
		return nil
	}

	return json.Unmarshal(ctx.Request.Data, target)
}
