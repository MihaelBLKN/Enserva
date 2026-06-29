package network

import (
	"encoding/json"
	"time"
)

const (
	defaultTickRate      = 128
	defaultSnapshotRate  = 20
	defaultClientTimeout = 5 * time.Second
)

type Config struct {
	UDPAddress    string
	TickRate      int
	SnapshotRate  int
	ClientTimeout time.Duration
}

func DefaultConfig() Config {
	return Config{
		UDPAddress:    ":9000",
		TickRate:      defaultTickRate,
		SnapshotRate:  defaultSnapshotRate,
		ClientTimeout: defaultClientTimeout,
	}
}

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

	return config
}

func (config Config) TickInterval() time.Duration {
	config = config.Normalized()

	return time.Second / time.Duration(config.TickRate)
}

func (config Config) SnapshotEvery() uint64 {
	config = config.Normalized()

	every := config.TickRate / config.SnapshotRate
	if every <= 0 {
		return 1
	}

	return uint64(every)
}

type Object interface {
	ObjectType() string
	ObjectID() string
	Snapshot() any
}

type TickHandler interface {
	OnTick(TickContext)
}

type FullTickHandler interface {
	OnFullTick(TickContext)
}

type RequestHandler interface {
	OnRequest(RequestContext) error
}

type AuthenticationHandler interface {
	OnAuthenticationAttempt(AuthenticationContext) (string, error)
}

type SnapshotVisibility interface {
	SnapshotVisible() bool
}

type ObjectFactory interface {
	CreateObject(RequestContext) (Object, error)
}

type ObjectFactoryFunc func(RequestContext) (Object, error)

func (factory ObjectFactoryFunc) CreateObject(ctx RequestContext) (Object, error) {
	return factory(ctx)
}

type RequestMessage struct {
	Type       string          `json:"type,omitempty"`
	Sequence   uint64          `json:"seq,omitempty"`
	ObjectType string          `json:"objectType"`
	ObjectID   string          `json:"objectId"`
	Action     string          `json:"action,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
}

type SnapshotData map[string]map[string]any

type SnapshotMessage struct {
	Type         string       `json:"type"`
	ClientID     string       `json:"clientId,omitempty"`
	Tick         uint64       `json:"tick"`
	LastSequence uint64       `json:"lastSeq,omitempty"`
	Objects      SnapshotData `json:"objects"`
}

type ResponseMessage struct {
	Type     string `json:"type"`
	Sequence uint64 `json:"seq,omitempty"`
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	Data     any    `json:"data,omitempty"`
}

type ResponseWriter interface {
	Respond(message any) error
}

type ResponseWriterFunc func(message any) error

func (writer ResponseWriterFunc) Respond(message any) error {
	if writer == nil {
		return ErrResponsesUnsupported
	}

	return writer(message)
}

type AuthenticationResponse struct {
	Type            string `json:"type"`
	Sequence        uint64 `json:"seq,omitempty"`
	OK              bool   `json:"ok"`
	ClientID        string `json:"clientId"`
	AuthenticatedID string `json:"authenticatedId"`
}

type TickContext struct {
	Tick         uint64
	Delta        time.Duration
	DeltaSeconds float64
	Runtime      *Runtime
}

type RequestContext struct {
	Transport  string
	ClientID   string
	Tick       uint64
	ReceivedAt time.Time
	Request    RequestMessage
	Runtime    *Runtime
	Response   ResponseWriter
}

func (ctx RequestContext) Decode(target any) error {
	if len(ctx.Request.Data) == 0 {
		return nil
	}

	return json.Unmarshal(ctx.Request.Data, target)
}

func (ctx RequestContext) Respond(message any) error {
	if ctx.Response == nil {
		return ErrResponsesUnsupported
	}

	return ctx.Response.Respond(message)
}

type AuthenticationContext struct {
	Transport    string
	ConnectionID string
	ClientID     string
	Tick         uint64
	ReceivedAt   time.Time
	Request      RequestMessage
	Runtime      *Runtime
}

func (ctx AuthenticationContext) Decode(target any) error {
	if len(ctx.Request.Data) == 0 {
		return nil
	}

	return json.Unmarshal(ctx.Request.Data, target)
}
