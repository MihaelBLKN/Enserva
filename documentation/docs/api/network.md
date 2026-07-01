# Package `network`

`Enserva/network` is the core package. It owns the runtime object registry, tick advancement, request routing, authentication hook, snapshot generation, binary wire protocol, and UDP server.

=== "GoLang"

    ```go
    import "Enserva/network"
    ```

=== "C#"

    ```csharp
    using Enserva.Network;
    ```

## Configuration

### `Config`

=== "GoLang"

    ```go
    type Config struct {
    	UDPAddress           string
    	TickRate             int
    	SnapshotRate         int
    	EnableDeltaSnapshots bool
    	FullSnapshotInterval int
    	ClientTimeout        time.Duration
    	MaxUDPPacketSize     int
    	ReliableRetryInterval time.Duration
    	ReliableMaxAttempts   int
    	ReliableQueueLimit    int
    	MaxInputFutureTicks   uint64
    	MaxInputPastTicks     uint64
    	InputBufferLimit      int
    	DebugEnabled         bool
    	DebugAddress         string
    }
    ```

=== "C#"

    ```csharp
    public sealed class EnservaConfig
    {
        public string UdpAddress { get; set; } = ":9000";
        public int TickRate { get; set; } = 128;
        public int SnapshotRate { get; set; } = 20;
        public bool EnableDeltaSnapshots { get; set; }
        public int FullSnapshotInterval { get; set; } = 64;
        public TimeSpan ClientTimeout { get; set; } = TimeSpan.FromSeconds(5);
        public int MaxUdpPacketSize { get; set; } = 1200;
        public TimeSpan ReliableRetryInterval { get; set; } = TimeSpan.FromMilliseconds(100);
        public int ReliableMaxAttempts { get; set; } = 5;
        public int ReliableQueueLimit { get; set; } = 64;
        public ulong MaxInputFutureTicks { get; set; } = 8;
        public ulong MaxInputPastTicks { get; set; } = 2;
        public int InputBufferLimit { get; set; } = 256;
        public bool DebugEnabled { get; set; }
        public string DebugAddress { get; set; } = ":9100";
    }
    ```

`Config` controls both the runtime and UDP server. Use `DefaultConfig()` when you want the repository defaults, then override fields:

=== "GoLang"

    ```go
    config := network.DefaultConfig()
config.UDPAddress = ":9100"
config.TickRate = 60
config.SnapshotRate = 10
config.EnableDeltaSnapshots = true
config.FullSnapshotInterval = 32
config.MaxUDPPacketSize = 1200
config.ReliableRetryInterval = 100 * time.Millisecond
config.ReliableMaxAttempts = 5
config.ReliableQueueLimit = 64
config.MaxInputFutureTicks = 8
config.MaxInputPastTicks = 2
config.InputBufferLimit = 256
    ```

=== "C#"

    ```csharp
    var config = EnservaConfig.Default();
config.UdpAddress = ":9100";
config.TickRate = 60;
config.SnapshotRate = 10;
config.EnableDeltaSnapshots = true;
config.FullSnapshotInterval = 32;
config.MaxUdpPacketSize = 1200;
config.ReliableRetryInterval = TimeSpan.FromMilliseconds(100);
config.ReliableMaxAttempts = 5;
config.ReliableQueueLimit = 64;
config.MaxInputFutureTicks = 8;
config.MaxInputPastTicks = 2;
config.InputBufferLimit = 256;

    var server = new EnservaServer(config);
    ```

Methods:

| Method                | Returns         | Notes                                                   |
| --------------------- | --------------- | ------------------------------------------------------- |
| `DefaultConfig()`     | `Config`        | `:9000`, `128` ticks/s, `20` snapshots/s, delta snapshots disabled, full interval `64`, `5s` timeout, `1200` byte UDP packet limit, reliable retry `100ms`, max attempts `5`, queue limit `64`, input future `8`, input past `2`, input buffer limit `256`, debug at `:9100` when enabled. |
| `Normalized()`        | `Config`        | Applies defaults, clamps snapshot rate to tick rate, and clamps invalid UDP packet, reliable-delivery, and input-buffer limits. |
| `TickInterval()`      | `time.Duration` | Duration between calls to `Runtime.Advance`.            |
| `SnapshotEvery()`     | `uint64`        | Tick interval between UDP snapshot broadcasts.          |
| `FullSnapshotEvery()` | `uint64`        | Normalized full snapshot interval for delta baseline cycles. |

## Core Object Interfaces

### `Object`

=== "GoLang"

    ```go
    type Object interface {
    	ObjectType() string
    	ObjectID() string
    	Snapshot() any
    }
    ```

=== "C#"

    ```csharp
    public interface IEnservaObject
    {
        string ObjectType { get; }
        string ObjectId { get; }
        object Snapshot();
    }
    ```

Every registered object must provide a type, an ID, and a serializable snapshot. Object type and ID are trimmed before use.

### Optional Hooks

| Interface               | Method                                                           | Purpose                                                         |
| ----------------------- | ---------------------------------------------------------------- | --------------------------------------------------------------- |
| `InitHandler`           | `OnInit(InitContext)`                                            | Called immediately after an object is registered.               |
| `TickHandler`           | `OnTick(TickContext)`                                            | Called every tick after the runtime increments its tick number. |
| `FullTickHandler`       | `OnFullTick(TickContext)`                                        | Called when `tick % TickRate == 0`.                             |
| `RequestHandler`        | `OnRequest(RequestContext) error`                                | Called for requests targeting an existing object.               |
| `AuthenticationHandler` | `OnAuthenticationAttempt(AuthenticationContext) (string, error)` | Called for wire `ClientHello` and legacy JSON authentication messages. |
| `SnapshotVisibility`    | `SnapshotVisible() bool`                                         | Return `false` to exclude an object from snapshots.             |
| `SceneSwitchHandler`    | `OnSceneSwitchRequest(SceneSwitchContext) (SceneSwitchDecision, error)` | Called for standard scene-switch requests targeting the object. |

## Factories

### `ObjectFactory`

=== "GoLang"

    ```go
    type ObjectFactory interface {
    	CreateObject(RequestContext) (Object, error)
    }
    ```

=== "C#"

    ```csharp
    public interface IObjectFactory
    {
        IEnservaObject CreateObject(RequestContext context);
    }
    ```

Factories are server-side helpers. Registering a factory does not allow a client to create an object by sending a request to a missing object.

### `ObjectFactoryFunc`

=== "GoLang"

    ```go
    type ObjectFactoryFunc func(RequestContext) (Object, error)
    ```

=== "C#"

    ```csharp
    public delegate IEnservaObject ObjectFactoryFunc(RequestContext context);
    ```

`ObjectFactoryFunc` adapts a function to `ObjectFactory`:

=== "GoLang"

    ```go
    server.RegisterFactory("player", network.ObjectFactoryFunc(func(ctx network.RequestContext) (network.Object, error) {
    	return NewPlayer(ctx.Request.ObjectID), nil
    }))
    ```

=== "C#"

    ```csharp
    server.RegisterFactory("player", context =>
    {
        return new Player(context.Request.ObjectId);
    });
    ```

## Runtime

### `NewRuntime`

=== "GoLang"

    ```go
    runtime := network.NewRuntime(network.Config{})
    ```

=== "C#"

    ```csharp
    var runtime = new EnservaRuntime(new EnservaConfig());
    ```

Creates a runtime with normalized configuration and empty object/factory maps.

### Object Management

| Method                                                            | Purpose                                                             |
| ----------------------------------------------------------------- | ------------------------------------------------------------------- |
| `RegisterObject(object Object) error`                             | Adds or replaces an object at its `ObjectType()/ObjectID()` key.    |
| `RegisterAuthenticationObject(object Object) error`               | Registers an object and binds it as the single auth handler.        |
| `RemoveObject(objectType, objectID string)`                       | Removes an object. Removing the auth object unbinds authentication. |
| `GetObject(objectType, objectID string) (Object, bool)`           | Looks up a registered object.                                       |
| `RegisterFactory(objectType string, factory ObjectFactory) error` | Registers a factory for server-side object creation.                |
| `CreateObject(objectType, objectID string) (Object, error)`       | Creates and registers an object through a registered factory.       |
| `RegisterWireMessage(definition WireMessageDefinition) error`     | Registers a custom binary message definition for this runtime.      |
| `WireMessages() *WireMessageRegistry`                             | Returns the runtime's protocol message registry.                    |

`CreateObject` validates that the factory returns an object with the requested type and ID.

=== "GoLang"

    ```go
    runtime := network.NewRuntime(network.Config{})
    if err := runtime.RegisterFactory("building", network.ObjectFactoryFunc(BuildingFactory)); err != nil {
    	return err
    }

    object, err := runtime.CreateObject("building", "building-1")
    if err != nil {
    	return err
    }
    _ = object
    ```

=== "C#"

    ```csharp
    var runtime = new EnservaRuntime(new EnservaConfig());
    runtime.RegisterFactory("building", BuildingFactory.Create);

    IEnservaObject building = runtime.CreateObject("building", "building-1");
    ```

### Simulation and Requests

| Method                                                                   | Purpose                                                                                 |
| ------------------------------------------------------------------------ | --------------------------------------------------------------------------------------- |
| `Advance() uint64`                                                       | Increments the tick, calls `OnTick`, and calls `OnFullTick` once per configured second. |
| `HandleRequest(ctx RequestContext) error`                                | Routes a request to the existing target object.                                         |
| `HandleAuthenticationAttempt(ctx AuthenticationContext) (string, error)` | Invokes the registered authentication handler.                                          |
| `RequestSceneSwitch(ctx SceneSwitchContext) (SceneSwitchDecision, error)` | Validates and applies a server-owned scene switch.                                      |
| `Snapshot() SnapshotData`                                                | Builds the nested snapshot map for visible objects.                                     |
| `SnapshotForClient(clientID string) SnapshotData`                        | Builds a client-specific snapshot when interest management is enabled.                  |
| `DebugState() DebugRuntimeState`                                         | Builds a full debug snapshot including hidden objects, factories, and auth state.       |
| `Tick() uint64`                                                          | Returns the current runtime tick.                                                       |
| `AuthenticationRequired() bool`                                          | Reports whether an auth handler is registered.                                          |
| `Config() Config`                                                        | Returns the normalized config.                                                          |
| `Features() *Features`                                                   | Returns the runtime feature registry.                                                   |

`HandleRequest` fills `ReceivedAt`, `Tick`, and `Runtime` on the context before invoking the object handler. Requests whose action is `scene.switch` are routed directly to `OnSceneSwitchRequest` instead of `OnRequest`.

### Input Buffering

| Method                                                                                              | Purpose                                                                  |
| --------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| `BufferClientInput(input ClientInput) error`                                                        | Validates and stores an input for its target tick.                       |
| `ConsumeClientInputs(clientID string) []ClientInput`                                                | Returns and removes inputs for the current runtime tick.                 |
| `ConsumeClientInputsForTick(clientID string, tick uint64) []ClientInput`                            | Returns and removes inputs for an explicit tick.                         |
| `ConsumeClientInputsForObject(clientID, objectType, objectID string) []ClientInput`                 | Returns and removes current-tick inputs targeting an object.             |
| `ConsumeClientInputsForObjectAtTick(clientID string, tick uint64, objectType, objectID string) []ClientInput` | Returns and removes object-targeted inputs for an explicit tick. |
| `InputBufferMetrics() InputBufferMetrics`                                                           | Returns cumulative buffered, consumed, rejected, and dropped counters.   |

`ClientInput` carries `ClientID`, input `Sequence`, target `Tick`, optional object or target IDs, an opaque `Payload`, and `ReceivedAt`. `BufferClientInput` rejects inputs outside `Config.MaxInputPastTicks` and `Config.MaxInputFutureTicks`; accepted inputs are returned deterministically by tick and sequence. The buffer is infrastructure only: game code decides how to interpret payloads and apply behavior.

=== "GoLang"

    ```go
    func (player *Player) OnTick(ctx network.TickContext) {
    	for _, input := range ctx.Runtime.ConsumeClientInputsForObject(
    		player.OwnerClientID,
    		player.ObjectType(),
    		player.ObjectID(),
    	) {
    		_ = input.Payload // Decode/apply in game code.
    	}
    }
    ```

=== "GoLang"

    ```go
    err := runtime.HandleRequest(network.RequestContext{
    	ClientID: "player-1",
    	Request: network.RequestMessage{
    		ObjectType: "player",
    		ObjectID:   "player-1",
    		Action:     "input",
    		Data:       json.RawMessage(`{"x":1,"y":0}`),
    	},
    })
    ```

=== "C#"

    ```csharp
    runtime.HandleRequest(new RequestContext
    {
        ClientId = "player-1",
        Request = new RequestMessage
        {
            ObjectType = "player",
            ObjectId = "player-1",
            Action = "input",
            DataJson = """{ "x": 1, "y": 0 }""",
        },
    });
    ```

## Server

### `Server`

`Server` is a small facade over `Runtime` plus transport startup.

| Function or method                                               | Purpose                                                        |
| ---------------------------------------------------------------- | -------------------------------------------------------------- |
| `NewServer(config Config) *Server`                               | Creates a server with a new runtime.                           |
| `ListenAndServe(config Config) error`                            | Convenience function for `NewServer(config).ListenAndServe()`. |
| `Config() Config`                                                | Returns normalized configuration.                              |
| `Runtime() *Runtime`                                             | Exposes the underlying runtime.                                |
| `RegisterObject`, `RegisterAuthenticationObject`, `RemoveObject` | Delegate to the runtime.                                       |
| `RegisterFactory`, `CreateObject`                                | Delegate factory operations to the runtime.                    |
| `RegisterWireMessage`                                             | Delegate custom binary message registration to the runtime.     |
| `ListenAndServe() error`                                         | Starts the UDP listener.                                       |
| `ListenAndServeUDP() error`                                      | Starts the UDP listener explicitly.                            |
| `ListenAndServeDebug() error`                                    | Starts only the debug HTTP listener.                           |
| `DebugHandler() http.Handler`                                    | Returns the debug UI and JSON API handler.                     |
| `DebugState() DebugState`                                        | Returns the current server debug snapshot.                     |
| `UDPServer() *UDPServer`                                         | Returns the active UDP server after startup, when available.   |

When `Config.DebugEnabled` is true, `ListenAndServeUDP` starts the debug HTTP listener in a goroutine before serving UDP. The UI is available at `/` and `/debug`, while `/debug/state` returns JSON containing normalized config, runtime state, feature state, UDP clients, transport counters, and object snapshots.

## UDP Server

### `NewUDPServer`

=== "GoLang"

    ```go
    udpServer := network.NewUDPServer(runtime)
    err := udpServer.ListenAndServe()
    ```

=== "C#"

    ```csharp
    var udpServer = new EnservaUdpServer(runtime);
    await udpServer.ListenAndServeAsync();
    ```

`UDPServer` accepts binary wire packets as the primary client protocol and legacy JSON datagrams for compatibility, tracks clients by UDP address, rejects duplicate or older non-zero sequence numbers, suppresses duplicate reliable message IDs, preserves ordered reliable dispatch, advances the runtime in a goroutine, and broadcasts snapshots at the configured rate. Serialized outbound responses and snapshots are dropped before `WriteToUDP` when they exceed `Config.MaxUDPPacketSize`.

When `Config.EnableDeltaSnapshots` is true, the UDP server stores per-client baselines and sends `WorldDeltaSnapshot` or `DeltaSnapshotMessage` after the first full snapshot until `FullSnapshotInterval` forces a new full baseline. Delta baselines reset after authentication changes, scene-switch handling, protocol mode changes, timeout removal, or reconnect.

Reliable wire responses stay in a per-client retry queue until a packet carrying them is acknowledged or the configured attempt limit is reached. This queue is opt-in; snapshots and legacy JSON traffic remain unreliable by default.

!!! warning
`UDPClient` and `UDPServer` expose no public fields. Treat their internals as implementation details.

## Message Types

### `RequestMessage`

=== "GoLang"

    ```go
    type RequestMessage struct {
    	Type       string          `json:"type,omitempty"`
    	Sequence   uint64          `json:"seq,omitempty"`
    	ObjectType string          `json:"objectType"`
    	ObjectID   string          `json:"objectId"`
    	Action     string          `json:"action,omitempty"`
    	Data       json.RawMessage `json:"data,omitempty"`
    }
    ```

=== "C#"

    ```csharp
    public sealed class RequestMessage
    {
        public string? Type { get; set; }
        public ulong Sequence { get; set; }
        public string ObjectType { get; set; } = "";
        public string ObjectId { get; set; } = "";
        public string? Action { get; set; }
        public string? DataJson { get; set; }
    }
    ```

Used for legacy JSON authentication/object requests and as the compatibility envelope that some built-in wire messages adapt into before reaching object handlers.

### `SnapshotData`, `SnapshotMessage`, and Deltas`

=== "GoLang"

    ```go
    type SnapshotData map[string]map[string]any
    ```

=== "C#"

    ```csharp
    using SnapshotData = Dictionary<string, Dictionary<string, object>>;
    ```

Snapshots are grouped by object type and object ID.

=== "GoLang"

    ```go
    type SnapshotMessage struct {
    	Type         string       `json:"type"`
    	ClientID     string       `json:"clientId,omitempty"`
    	Tick         uint64       `json:"tick"`
    	LastSequence uint64       `json:"lastSeq,omitempty"`
    	Objects      SnapshotData `json:"objects"`
    }
    ```

=== "C#"

    ```csharp
    public sealed class SnapshotMessage
    {
        public string Type { get; set; } = "snapshot";
        public string? ClientId { get; set; }
        public ulong Tick { get; set; }
        public ulong LastSequence { get; set; }
        public SnapshotData Objects { get; set; } = new();
    }
    ```

`SnapshotMessage` is the legacy JSON snapshot envelope. Wire clients receive the equivalent state through `WorldSnapshot`.

Delta snapshots use the same object grouping for spawned and changed objects:

=== "GoLang"

    ```go
    type SnapshotObjectRef struct {
    	ObjectType string `json:"objectType"`
    	ObjectID   string `json:"objectId"`
    }

    type SnapshotDelta struct {
    	Spawned   SnapshotData        `json:"spawned,omitempty"`
    	Changed   SnapshotData        `json:"changed,omitempty"`
    	Despawned []SnapshotObjectRef `json:"despawned,omitempty"`
    }

    type DeltaSnapshotMessage struct {
    	Type         string              `json:"type"`
    	ClientID     string              `json:"clientId,omitempty"`
    	Tick         uint64              `json:"tick"`
    	LastSequence uint64              `json:"lastSeq,omitempty"`
    	BaselineTick uint64              `json:"baselineTick,omitempty"`
    	Spawned      SnapshotData        `json:"spawned,omitempty"`
    	Changed      SnapshotData        `json:"changed,omitempty"`
    	Despawned    []SnapshotObjectRef `json:"despawned,omitempty"`
    }
    ```

| Function or method                    | Purpose                                                                    |
| ------------------------------------- | -------------------------------------------------------------------------- |
| `BuildSnapshotDelta(previous, current)` | Compares visible snapshots and returns spawned, changed, and despawned data. |
| `CloneSnapshotData(snapshot)`         | Returns a detached JSON-shaped copy for storing baselines.                 |
| `SnapshotDelta.Empty()`               | Reports whether a delta contains no changes.                               |

### `ResponseMessage`

=== "GoLang"

    ```go
    type ResponseMessage struct {
    	Type     string `json:"type"`
    	Sequence uint64 `json:"seq,omitempty"`
    	OK       bool   `json:"ok"`
    	Error    string `json:"error,omitempty"`
    	Data     any    `json:"data,omitempty"`
    }
    ```

=== "C#"

    ```csharp
    public sealed class ResponseMessage
    {
        public string Type { get; set; } = "";
        public ulong Sequence { get; set; }
        public bool Ok { get; set; }
        public string? Error { get; set; }
        public object? Data { get; set; }
    }
    ```

Used by the UDP transport for legacy JSON error responses and available to object handlers through `RequestContext.Respond`. Wire responses are encoded through registered server messages such as `ErrorMessage`.

If a UDP response serializes beyond `Config.MaxUDPPacketSize`, the transport drops it and returns `ErrUDPPacketTooLarge` from the response writer.

### `AuthenticationResponse`

=== "GoLang"

    ```go
    type AuthenticationResponse struct {
    	Type            string `json:"type"`
    	Sequence        uint64 `json:"seq,omitempty"`
    	OK              bool   `json:"ok"`
    	ClientID        string `json:"clientId"`
    	AuthenticatedID string `json:"authenticatedId"`
    }
    ```

=== "C#"

    ```csharp
    public sealed class AuthenticationResponse
    {
        public string Type { get; set; } = "auth";
        public ulong Sequence { get; set; }
        public bool Ok { get; set; }
        public string ClientId { get; set; } = "";
        public string AuthenticatedId { get; set; } = "";
    }
    ```

Returned by the UDP server after successful legacy JSON authentication. Wire clients receive `Welcome`.

## Wire Protocol API

The UDP transport accepts binary packets that start with `WireProtocolMagic` and `WireProtocolVersion`. See [Wire Protocol](wire-protocol.md) for the packet layout.

### Constants

| Constant                         | Value range or meaning                                      |
| -------------------------------- | ----------------------------------------------------------- |
| `WireProtocolMagic`              | `0x4553`, the ASCII bytes `ES`.                             |
| `WireProtocolVersion`            | Current binary packet version.                              |
| `MaxWirePayloadSize`             | Maximum packet payload size accepted by the encoder.        |
| `MaxWireMessagePayloadSize`      | Maximum payload size for one framed message.                |
| `MaxWireMessagesPerPacket`       | Maximum framed messages accepted in one packet.             |
| `MaxWireStringBytes`             | Maximum string field size for most built-in wire messages.  |
| `MaxWireChatBytes`               | Maximum string size for the reserved chat message codec.    |
| `WireMessageSystemMin`, `WireMessageSystemMax` | Reserved system message ID range, `0x0000-0x00ff`. |
| `WireMessageEngineMin`, `WireMessageEngineMax` | Built-in engine message ID range, `0x0100-0x0fff`. |
| `WireMessageGameMin`, `WireMessageGameMax` | Custom game message ID range, `0x1000-0xffff`. |

### Built-In Message IDs

| Message ID constant             | Typed payload          | Direction                  |
| ------------------------------- | ---------------------- | -------------------------- |
| `WireMessageUnknown`            | `UnknownWireMessage`   | Preserved and skipped.     |
| `WireMessageClientHello`        | `ClientHello`          | Client to server.          |
| `WireMessageWelcome`            | `Welcome`              | Server to client.          |
| `WireMessagePing`               | `Ping`                 | Client to server.          |
| `WireMessagePong`               | `Pong`                 | Server to client.          |
| `WireMessageError`              | `ErrorMessage`         | Server to client.          |
| `WireMessageDisconnect`         | `DisconnectMessage`    | Server to client.          |
| `WireMessageReliable`           | Reliable envelope      | Protocol wrapper.          |
| `WireMessageObjectRequest`      | `ObjectRequest`        | Client to server.          |
| `WireMessagePlayerInput`        | `PlayerInput`          | Client to server.          |
| `WireMessageWorldSnapshot`      | `WorldSnapshot`        | Server to client.          |
| `WireMessageDeltaSnapshot`      | `WorldDeltaSnapshot`   | Server to client.          |
| `WireMessageClientInput`        | `GenericClientInput`   | Client to server.          |
| `WireMessageEntitySpawn`        | `EntitySpawn`          | Server to client.          |
| `WireMessageEntityDespawn`      | `EntityDespawn`        | Server to client.          |
| `WireMessageEntityUpdate`       | `EntityUpdate`         | Server to client.          |

### Packet Helpers

| Function                                                          | Purpose                                                            |
| ----------------------------------------------------------------- | ------------------------------------------------------------------ |
| `EncodePacket(sequence uint64, messages []WireMessage)`           | Frames encoded messages with zero acknowledgement fields.          |
| `EncodePacketWithAcks(sequence, ack, ackBits uint64, messages []WireMessage)` | Frames encoded messages with acknowledgement state.       |
| `DecodePacket(data []byte) (WirePacket, error)`                   | Validates and splits a binary packet into framed messages.         |
| `EncodeClientMessage(message any) (WireMessage, error)`           | Encodes a typed client message using the default registry.         |
| `DecodeClientMessage(message WireMessage) (any, error)`           | Decodes one framed client message using the default registry.      |
| `EncodeServerMessage(message any) (WireMessage, error)`           | Encodes a typed server message using the default registry.         |
| `DecodeServerMessage(message WireMessage) (any, error)`           | Decodes one framed server message using the default registry.      |

### Registry Types

`WireMessageRegistry` stores schemas by numeric ID and Go message type. Runtime instances own separate registries, while package-level helpers use `DefaultWireMessages()`.

=== "GoLang"

    ```go
    type WireMessageDefinition struct {
    	ID          WireMessageType
    	Name        string
    	Direction   WireMessageDirection
    	Delivery    DeliveryClass
    	MessageType reflect.Type
    	Encode      WireMessageEncoder
    	Decode      WireMessageDecoder
    	Validate    WireMessageValidator
    	Handler     WireMessageHandler
    }
    ```

=== "C#"

    ```csharp
    public sealed class WireMessageDefinition
    {
        public ushort Id { get; init; }
        public string Name { get; init; } = "";
        public WireMessageDirection Direction { get; init; }
        public DeliveryClass Delivery { get; init; }
        public Type MessageType { get; init; } = typeof(object);
        public Func<object, byte[]> Encode { get; init; } = _ => Array.Empty<byte>();
        public Func<byte[], object> Decode { get; init; } = _ => new object();
        public Action<object>? Validate { get; init; }
        public Func<WireMessageContext, Task>? Handler { get; init; }
    }
    ```

| Method or function                                      | Purpose                                                        |
| ------------------------------------------------------- | -------------------------------------------------------------- |
| `NewWireMessageRegistry()`                              | Creates an empty registry.                                     |
| `NewDefaultWireMessageRegistry()`                       | Creates a registry with Enserva's built-in messages.           |
| `DefaultWireMessages()`                                 | Returns the process-wide default registry.                     |
| `Register(definition WireMessageDefinition) error`      | Adds one message schema.                                       |
| `Definition(id WireMessageType) (WireMessageDefinition, bool)` | Looks up a schema by ID.                              |
| `EncodeMessage(message any) (WireMessage, error)`       | Encodes a typed message by Go type.                            |
| `DecodeMessage(message WireMessage) (any, error)`       | Decodes a framed message or returns `UnknownWireMessage`.      |
| `Dispatch(ctx WireMessageContext) (bool, error)`        | Calls a registered handler when one exists.                    |

`WireMessageContext` gives handlers access to transport, client, sequence, ack, message, runtime, and response writer fields.

### Delivery Helpers

| Type or function | Purpose |
| --- | --- |
| `DeliveryClass` | Delivery metadata for wire messages. Values are `DeliveryUnreliable`, `DeliveryReliableOrdered`, and `DeliveryReliableUnordered`. |
| `WireDelivery` | Wrapper accepted by UDP response writers to override the delivery class for one message. |
| `Deliver(message, class)` | Wraps a response with an explicit delivery class. |
| `DeliverReliableOrdered(message)` | Sends a response as reliable ordered when the peer uses wire packets. |
| `DeliverReliableUnordered(message)` | Sends a response as reliable unordered when the peer uses wire packets. |
| `DeliverUnreliable(message)` | Explicitly keeps a response unreliable. |

Reliable delivery is only applied to binary wire messages. Snapshot messages are encoded with the default unreliable class unless an application explicitly changes its own message metadata.

## Features

### Interest Management

Interest management is configured through `Runtime.Features()`:

=== "GoLang"

    ```go
    func (player *Player) OnInit(ctx network.InitContext) {
    	ctx.Runtime().Features().EnableInterestManagement(
    		network.PlayerInterest(player, "x", "y", "z", 750),
    	)
    }
    ```

=== "C#"

    ```csharp
    public void OnInit(InitContext ctx)
    {
        ctx.Runtime.Features.EnableInterestManagement(
            Interest.Player(this, "x", "y", "z", radius: 750));
    }
    ```

Helper functions:

| Function                                          | Purpose                                      |
| ------------------------------------------------- | -------------------------------------------- |
| `PlayerInterest(object, x, y, z, radius)`         | Registers a 3D player/reference object.      |
| `PlayerInterest2D(object, x, y, radius)`          | Registers a 2D player/reference object.      |
| `GameObjectInterest(object, x, y, z)`             | Registers a 3D object that can be filtered.  |
| `GameObjectInterest2D(object, x, y)`              | Registers a 2D object that can be filtered.  |
| `EnableInterestManagement(InterestManagementConfig)` | Stores interest metadata for one object. |

See [Interest Management](../features/interest-management.md) for a full guide.

### Scene Management

Scene management is configured through `Runtime.Features()` and filters snapshots by client scene:

=== "GoLang"

    ```go
    features := runtime.Features()
    features.SetClientScene("player-1", "arena-a")
    features.SetObjectScene("player", "player-1", "arena-a")
    features.SetObjectGlobal("match", "scoreboard")
    ```

=== "C#"

    ```csharp
    var features = runtime.Features;
    features.SetClientScene("player-1", "arena-a");
    features.SetObjectScene("player", "player-1", "arena-a");
    features.SetObjectGlobal("match", "scoreboard");
    ```

Scene helpers:

| Method or constant | Purpose |
| --- | --- |
| `SceneID` | Identifies a server-owned scene, room, map, shard, or phase. |
| `SceneGlobal` | Marks an object as visible in every scene. |
| `Features.Scenes()` | Returns the scene manager, creating it when needed. |
| `SetClientScene(clientID, sceneID)` | Assigns a client to the scene used for snapshot filtering. |
| `SetObjectScene(objectType, objectID, sceneID)` | Assigns an object to a scene. |
| `SetObjectSceneForObject(object, sceneID)` | Assigns a registered object using its object identity. |
| `SetObjectGlobal(objectType, objectID)` | Marks an object as globally visible across scenes. |
| `ClearClientScene(clientID)` | Removes a client's explicit scene assignment. |
| `ClearObjectScene(objectType, objectID)` | Removes an object's explicit scene assignment. |
| `ClientScene(clientID)` | Reads a client's assigned scene. |
| `ObjectScene(objectType, objectID)` | Reads an object's assigned scene. |

See [Scenes](../guides/scenes.md) for a full guide.

## Context Types

### `InitContext`

| Method         | Description                                      |
| -------------- | ------------------------------------------------ |
| `Object()`     | Object being initialized.                        |
| `ObjectType()` | Normalized object type used for registration.    |
| `ObjectID()`   | Normalized object ID used for registration.      |
| `Runtime()`    | Runtime that just registered the object.         |

### `TickContext`

| Field          | Description               |
| -------------- | ------------------------- |
| `Tick`         | Current tick number.      |
| `Delta`        | Tick duration.            |
| `DeltaSeconds` | Tick duration as seconds. |
| `Runtime`      | Runtime calling the hook. |
| `Features`     | Runtime feature registry. |

### `RequestContext`

| Field        | Description                                                    |
| ------------ | -------------------------------------------------------------- |
| `Transport`  | Transport name, such as `"udp"` in the built-in UDP server.    |
| `ClientID`   | Client identity assigned by the transport/authentication flow. |
| `Tick`       | Runtime tick when the request is routed.                       |
| `ReceivedAt` | Request timestamp.                                             |
| `Request`    | Parsed request message.                                        |
| `Payload`    | Protocol-decoded payload for binary messages when available.    |
| `Runtime`    | Runtime routing the request.                                   |
| `Features`   | Runtime feature registry.                                      |
| `Response`   | Optional response writer.                                      |

Methods:

| Method                       | Purpose                                                              |
| ---------------------------- | -------------------------------------------------------------------- |
| `Decode(target any) error`   | Decodes binary `Payload` or JSON `Request.Data` into `target`.       |
| `Respond(message any) error` | Sends a direct response when supported.                              |
| `RequestSceneSwitch(sceneID) (SceneSwitchDecision, error)` | Applies a scene switch from custom request code. Standard `scene.switch` requests use `OnSceneSwitchRequest` directly. |

### `SceneSwitchContext`

`SceneSwitchContext` is passed to objects that implement `SceneSwitchHandler`.

| Field          | Description                                                    |
| -------------- | -------------------------------------------------------------- |
| `Transport`    | Transport name, such as `"udp"` in the built-in UDP server.    |
| `ClientID`     | Client identity assigned by the transport/authentication flow. |
| `Tick`         | Runtime tick when the request is routed.                       |
| `ReceivedAt`   | Request timestamp.                                             |
| `Request`      | Parsed request message that triggered the switch.              |
| `Payload`      | Protocol-decoded payload for binary messages when available.    |
| `Object`       | Target object being asked to validate the switch.              |
| `ObjectType`   | Normalized target object type.                                 |
| `ObjectID`     | Normalized target object ID.                                   |
| `CurrentScene` | Current object scene before any mutation.                      |
| `TargetScene`  | Requested target scene after normalization.                    |
| `Runtime`      | Runtime routing the scene switch.                              |
| `Features`     | Runtime feature registry.                                      |
| `Response`     | Optional response writer.                                      |

Methods:

| Method                       | Purpose                                                              |
| ---------------------------- | -------------------------------------------------------------------- |
| `Decode(target any) error`   | Decodes binary `Payload` or JSON `Request.Data` into `target`.       |
| `Respond(message any) error` | Sends a direct response when supported.                              |

Scene switch values:

| Type or function | Purpose |
| --- | --- |
| `SceneSwitchDecision` | Reports whether a switch was allowed, final scene, reason, client-clear flag, and optional data. |
| `SceneSwitchAllowed()` | Allows the requested scene and sets `ClearClientObjects`. |
| `SceneSwitchAllowedTo(sceneID)` | Allows the switch but redirects to a different final scene. |
| `SceneSwitchDenied(reason)` | Denies the switch and preserves existing scene state. |
| `SceneSwitchRequest` | Standard request payload with `TargetScene`. |
| `SceneSwitchResponse` | Standard immediate response containing final/previous scene, reason, and optional data. |

### `AuthenticationContext`

Authentication context is similar to request context but carries `ConnectionID` and has no `ResponseWriter`.

| Field          | Description                                                     |
| -------------- | --------------------------------------------------------------- |
| `Transport`    | Transport name.                                                 |
| `ConnectionID` | Transport-level connection identity, such as a UDP address key. |
| `ClientID`     | Current client ID before authentication completes.              |
| `Tick`         | Runtime tick when authentication is routed.                     |
| `ReceivedAt`   | Authentication timestamp.                                       |
| `Request`      | Parsed request message.                                         |
| `Payload`      | Protocol-decoded payload for binary messages when available.     |
| `Runtime`      | Runtime routing the authentication attempt.                     |
| `Features`     | Runtime feature registry.                                       |

Method:

| Method                     | Purpose                                                              |
| -------------------------- | -------------------------------------------------------------------- |
| `Decode(target any) error` | Decodes binary `Payload` or JSON `Request.Data` into `target`.       |

## Response Writers

### `ResponseWriter`

=== "GoLang"

    ```go
    type ResponseWriter interface {
    	Respond(message any) error
    }
    ```

=== "C#"

    ```csharp
    public interface IResponseWriter
    {
        Task RespondAsync(object message);
    }
    ```

`RequestContext.Response` can be nil for non-transport tests or direct runtime calls.

### `ResponseWriterFunc`

`ResponseWriterFunc` adapts a function to `ResponseWriter`. A nil function returns `ErrResponsesUnsupported`.

## Error Values

These exported errors are intended for comparison with `errors.Is`:

| Error                                 | Raised when                                                                       |
| ------------------------------------- | --------------------------------------------------------------------------------- |
| `ErrMissingObjectType`                | An object or request lacks an object type.                                        |
| `ErrMissingObjectID`                  | An object or request lacks an object ID.                                          |
| `ErrObjectNotFound`                   | A target object or factory does not exist.                                        |
| `ErrObjectExists`                     | `CreateObject` is asked to create an existing object.                             |
| `ErrMissingAuthenticationHandler`     | Authentication is attempted with no registered handler.                           |
| `ErrAuthenticationHandlerExists`      | A second authentication object is registered.                                     |
| `ErrAuthenticationHandlerUnsupported` | Registered auth object does not implement `AuthenticationHandler`.                |
| `ErrAuthenticationRequired`           | An unauthenticated UDP client sends a regular request while auth is required.     |
| `ErrAuthenticatedClientIDInUse`       | A UDP client authenticates as an ID already used by another authenticated client. |
| `ErrMissingAuthenticationID`          | Authentication returns an empty ID.                                               |
| `ErrResponsesUnsupported`             | A response is attempted without a response writer.                                |
| `ErrUDPPacketTooLarge`                | A serialized outbound UDP response or snapshot exceeds `Config.MaxUDPPacketSize`. |
| `ErrInvalidDeliveryClass`             | A reliable envelope or message definition uses an unknown delivery class.         |
| `ErrReliableMessageID`                | A reliable wire message is missing its non-zero reliable ID.                      |
| `ErrReliableQueueFull`                | A UDP client already has `Config.ReliableQueueLimit` pending reliable messages.   |
| `ErrMissingSceneRuntime`              | A scene switch was requested without a runtime.                                   |
| `ErrMissingSceneID`                   | A scene switch request has an empty target scene.                                 |
| `ErrSceneSwitchUnsupported`           | The target object does not implement `SceneSwitchHandler`.                        |
| `ErrInvalidWirePacket`                | A binary packet has invalid framing, length, or message count.                    |
| `ErrUnsupportedWireVersion`           | A packet uses a protocol version this build does not support.                     |
| `ErrWirePacketTooLarge`               | A packet exceeds the configured wire payload size.                                |
| `ErrWireMessageTooLarge`              | One framed message exceeds the configured payload size.                           |
| `ErrWireStringTooLarge`               | A string field exceeds the configured wire string limit.                          |
| `ErrMalformedWirePayload`             | A typed wire message payload is truncated or malformed.                           |
| `ErrUnsupportedWireMessage`           | Encoding was requested for an unregistered message type.                          |
| `ErrUnsupportedWireSnapshotValue`     | Snapshot encoding found a value kind the wire format cannot represent.            |
| `ErrWireMessageRegistered`            | A wire message ID or Go type is already registered.                               |
| `ErrWireMessageTypeOutOfRange`        | A wire message ID is outside the known reserved ranges.                           |
| `ErrMissingWireMessageCodec`          | A wire message definition has no encoder or decoder.                              |
| `ErrMissingWireMessageName`           | A wire message definition has no name.                                            |
| `ErrMissingWireMessageType`           | A wire message definition has no Go message type.                                 |
| `ErrWireMessageValidation`            | A wire message validator rejected a decoded or encoded message.                   |
| `ErrMissingWireMessageRegistry`       | A nil wire registry was used.                                                     |
