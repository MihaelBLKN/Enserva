# Implementing Network Objects

Enserva includes a package named `Enserva/netObjects`, but that package is example application code. It shows how network objects can be implemented, registered, and tested.

The framework API lives in [`Enserva/network`](network.md). Your own project can create any package name you like for authoritative server objects.

!!! important
Do not treat the sample `Player`, `Building`, or `PlayerAuthenticator` behavior as Enserva framework behavior. They are examples of object implementations. The reusable contract is the set of interfaces and contexts from `network`.

## Basic Object Contract

Every object registered with Enserva must implement `network.Object`:

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

=== "Rust"

    ```rust
    trait EnservaObject {
        fn object_type(&self) -> &str;
        fn object_id(&self) -> &str;
        fn snapshot(&self) -> SnapshotValue;
    }

    #[derive(Clone, Debug)]
    enum SnapshotValue {
        Null,
        Bool(bool),
        Number(f64),
        String(String),
    }
    ```

| Method                | Purpose                                                                                                 |
| --------------------- | ------------------------------------------------------------------------------------------------------- |
| `ObjectType() string` | Returns the object category used for routing and snapshot grouping, such as `"player"` or `"building"`. |
| `ObjectID() string`   | Returns the unique object ID inside that object type.                                                   |
| `Snapshot() any`      | Returns the serializable state that should appear in snapshots.                                         |

The runtime trims object type and ID strings. Empty values are rejected.

## Optional Object Hooks

Objects can implement any of these methods. Enserva detects them through interfaces at runtime.

| Method                                                                   | Interface                       | When it runs                                                                                             |
| ------------------------------------------------------------------------ | ------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `OnInit(network.InitContext)`                                            | `network.InitHandler`           | Immediately after an object is registered with the runtime.                                              |
| `OnTick(network.TickContext)`                                            | `network.TickHandler`           | Every simulation tick after the runtime increments `Tick`.                                               |
| `OnFullTick(network.TickContext)`                                        | `network.FullTickHandler`       | Once per completed second of ticks, using `tick % TickRate == 0`.                                        |
| `OnRequest(network.RequestContext) error`                                | `network.RequestHandler`        | When a request targets an existing object with matching `objectType` and `objectId`.                     |
| `OnAuthenticationAttempt(network.AuthenticationContext) (string, error)` | `network.AuthenticationHandler` | When the transport receives a wire `ClientHello` or legacy JSON auth message and this object was registered as the authentication object. |
| `SnapshotVisible() bool`                                                 | `network.SnapshotVisibility`    | During snapshot generation. Return `false` for server-only objects.                                      |
| `OnSceneSwitchRequest(network.SceneSwitchContext) (network.SceneSwitchDecision, error)` | `network.SceneSwitchHandler` | When a standard scene-switch request targets the object.                                                 |

!!! note
An object can implement only the hooks it needs. For example, a static object may only implement `ObjectType`, `ObjectID`, and `Snapshot`.

## Server-Side Factories

Factories are not object methods. They are server-side creation helpers:

=== "GoLang"

    ```go
    type ObjectFactory interface {
    	CreateObject(network.RequestContext) (network.Object, error)
    }
    ```

=== "C#"

    ```csharp
    public interface IObjectFactory
    {
        IEnservaObject CreateObject(RequestContext context);
    }
    ```

=== "Rust"

    ```rust
    trait ObjectFactory {
        fn create_object(&self, context: &RequestContext) -> Box<dyn EnservaObject>;
    }

    struct RequestContext {
        object_id: String,
    }
    ```

Most simple factories use `network.ObjectFactoryFunc`:

=== "GoLang"

    ```go
    func ProjectileFactory(ctx network.RequestContext) (network.Object, error) {
    	return &Projectile{ID: ctx.Request.ObjectID}, nil
    }

    if err := server.RegisterFactory("projectile", network.ObjectFactoryFunc(ProjectileFactory)); err != nil {
    	return err
    }
    ```

=== "C#"

    ```csharp
    IEnservaObject ProjectileFactory(RequestContext ctx)
    {
        return new Projectile { Id = ctx.Request.ObjectId };
    }

    server.RegisterFactory("projectile", ProjectileFactory);
    ```

=== "Rust"

    ```rust
    fn projectile_factory(ctx: &RequestContext) -> Projectile {
        Projectile {
            id: ctx.object_id.clone(),
        }
    }

    server.register_factory("projectile", projectile_factory)?;
    ```

Client requests do not call factories. To create an object through a factory, server code must call:

=== "GoLang"

    ```go
    object, err := server.CreateObject("projectile", "projectile-1")
    ```

=== "C#"

    ```csharp
    IEnservaObject projectile = server.CreateObject("projectile", "projectile-1");
    ```

=== "Rust"

    ```rust
    let projectile = server.create_object("projectile", "projectile-1")?;
    ```

## Registration Methods

Use these `network.Server` or `network.Runtime` methods to make objects available:

| Method                                                                    | Use                                                                  |
| ------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| `RegisterObject(object network.Object) error`                             | Register an object that already exists.                              |
| `RegisterAuthenticationObject(object network.Object) error`               | Register an object and bind it as the single authentication handler. |
| `RegisterFactory(objectType string, factory network.ObjectFactory) error` | Register a server-side creation helper.                              |
| `CreateObject(objectType, objectID string) (network.Object, error)`       | Create an object through a registered factory.                       |
| `RemoveObject(objectType, objectID string)`                               | Remove an object from the runtime.                                   |
| `GetObject(objectType, objectID string) (network.Object, bool)`           | Look up a registered object.                                         |

## Minimal Object Example

=== "GoLang"

    ```go
    package world

    import "Enserva/network"

    type Door struct {
    	ID         string `json:"id"`
    	Open       bool   `json:"open"`
    	LastClient string `json:"lastClient,omitempty"`
    }

    func (door *Door) ObjectType() string {
    	return "door"
    }

    func (door *Door) ObjectID() string {
    	return door.ID
    }

    func (door *Door) Snapshot() any {
    	return *door
    }

    func (door *Door) OnRequest(ctx network.RequestContext) error {
    	switch ctx.Request.Action {
    	case "open":
    		door.Open = true
    	case "close":
    		door.Open = false
    	}

    	door.LastClient = ctx.ClientID
    	return nil
    }
    ```

=== "C#"

    ```csharp
    public sealed class Door : IEnservaObject
    {
        public string Id { get; init; } = "";
        public bool Open { get; private set; }
        public ulong Uses { get; private set; }
        public string? Client { get; private set; }

        public string ObjectType => "door";
        public string ObjectId => Id;
        public object Snapshot() => this;

        public void OnRequest(RequestContext ctx)
        {
            if (ctx.Request.Action == "open") Open = true;
            if (ctx.Request.Action == "close") Open = false;

            Uses++;
            Client = ctx.ClientId;
        }
    }
    ```

=== "Rust"

    ```rust
    #[derive(Clone, Debug)]
    struct Door {
        id: String,
        open: bool,
        uses: u64,
        client: Option<String>,
    }

    impl Door {
        fn object_type(&self) -> &str { "door" }
        fn object_id(&self) -> &str { &self.id }

        fn on_request(&mut self, ctx: &RequestContext) {
            match ctx.action.as_str() {
                "open" => self.open = true,
                "close" => self.open = false,
                _ => {}
            }
            self.uses += 1;
            self.client = Some(ctx.client_id.clone());
        }
    }
    ```

Register it:

=== "GoLang"

    ```go
    door := &world.Door{ID: "door-1"}
    if err := server.RegisterObject(door); err != nil {
    	return err
    }
    ```

=== "C#"

    ```csharp
    var door = new Door { Id = "door-1" };
    server.RegisterObject(door);
    ```

=== "Rust"

    ```rust
    let door = Door {
        id: "door-1".into(),
        open: false,
        uses: 0,
        client: None,
    };

    server.register_object(door)?;
    ```

## Init Hook Example

Use `OnInit` for setup that needs the runtime after an object is registered. Interest management is a common example:

=== "GoLang"

    ```go
    func (player *Player) OnInit(ctx network.InitContext) {
    	ctx.Runtime().Features().EnableInterestManagement(
    		network.PlayerInterest2D(player, "x", "y", 500),
    	)
    }
    ```

=== "C#"

    ```csharp
    public void OnInit(InitContext ctx)
    {
        ctx.Runtime.Features.EnableInterestManagement(
            Interest.Player2D(this, "x", "y", radius: 500));
    }
    ```

=== "Rust"

    ```rust
    fn on_init(&self, ctx: &mut InitContext) {
        ctx.runtime.features.enable_interest_management(InterestManagementConfig::player(
            self.object_type(),
            self.object_id(),
            "x",
            "y",
            Some("z"),
            750.0,
        ));
    }
    ```

## Tick Hook Example

=== "GoLang"

    ```go
    func (projectile *Projectile) OnTick(ctx network.TickContext) {
    	projectile.X += projectile.VelocityX * ctx.DeltaSeconds
    	projectile.Y += projectile.VelocityY * ctx.DeltaSeconds
    }
    ```

=== "C#"

    ```csharp
    public void OnTick(TickContext ctx)
    {
        X += VelocityX * ctx.DeltaSeconds;
        Y += VelocityY * ctx.DeltaSeconds;
    }
    ```

=== "Rust"

    ```rust
    fn on_tick(&mut self, ctx: &TickContext) {
        self.x += self.velocity_x * ctx.delta_seconds;
        self.y += self.velocity_y * ctx.delta_seconds;
    }
    ```

`TickContext` includes:

| Field          | Use                                 |
| -------------- | ----------------------------------- |
| `Tick`         | Current runtime tick.               |
| `Delta`        | Tick duration as `time.Duration`.   |
| `DeltaSeconds` | Tick duration as `float64` seconds. |
| `Runtime`      | Runtime invoking the hook.          |
| `Features`     | Runtime feature registry.           |

## Request Hook Basics

`OnRequest` receives a `network.RequestContext`:

=== "GoLang"

    ```go
    func (object *Thing) OnRequest(ctx network.RequestContext) error {
    	var payload struct {
    		Value int `json:"value"`
    	}
    	if err := ctx.Decode(&payload); err != nil {
    		return err
    	}

    	return nil
    }
    ```

=== "C#"

    ```csharp
    public void OnRequest(RequestContext ctx)
    {
        var payload = ctx.Decode<ThingRequest>();
        // Apply server-authoritative changes here.
    }

    public sealed record ThingRequest(int Value);
    ```

=== "Rust"

    ```rust
    struct ThingRequest {
        value: i32,
    }

    fn on_request(&mut self, ctx: &RequestContext) -> Result<(), Box<dyn std::error::Error>> {
        let payload: ThingRequest = ctx.decode()?;
        let _ = payload.value;
        Ok(())
    }
    ```

Useful fields and methods:

| Field or method          | Use                                                    |
| ------------------------ | ------------------------------------------------------ |
| `ctx.ClientID`           | Authenticated or transport-level client identity.      |
| `ctx.Request.ObjectType` | Target object type.                                    |
| `ctx.Request.ObjectID`   | Target object ID.                                      |
| `ctx.Request.Action`     | Object-defined action name.                            |
| `ctx.Decode(&target)`    | Decode the typed wire payload or legacy `ctx.Request.Data` JSON. |
| `ctx.Respond(message)`   | Send a direct response when the transport supports it. |
| `ctx.Runtime`            | Access the runtime routing the request.                |
| `ctx.Features`           | Access runtime feature configuration.                  |

## Authentication Object Basics

Authentication is handled by one normal object that implements `OnAuthenticationAttempt`.

=== "GoLang"

    ```go
    func (auth *Authenticator) OnAuthenticationAttempt(ctx network.AuthenticationContext) (string, error) {
    	var payload struct {
    		Token string `json:"token"`
    	}
    	if err := ctx.Decode(&payload); err != nil {
    		return "", err
    	}

    	accountID, err := authenticateToken(payload.Token)
    	if err != nil {
    		return "", err
    	}

    	return "player-" + accountID, nil
    }
    ```

=== "C#"

    ```csharp
    public string OnAuthenticationAttempt(AuthenticationContext ctx)
    {
        var payload = ctx.Decode<AuthPayload>();
        string accountId = AuthenticateToken(payload.Token);
        return $"player-{accountId}";
    }

    public sealed record AuthPayload(string Token);
    ```

=== "Rust"

    ```rust
    struct AuthPayload {
        token: String,
    }

    fn on_authentication_attempt(ctx: &AuthenticationContext) -> Result<String, AuthError> {
        let payload: AuthPayload = ctx.decode()?;
        let account_id = authenticate_token(&payload.token)?;
        Ok(format!("player-{account_id}"))
    }
    ```

Register it:

=== "GoLang"

    ```go
    if err := server.RegisterAuthenticationObject(authenticator); err != nil {
    	return err
    }
    ```

=== "C#"

    ```csharp
    server.RegisterAuthenticationObject(new Authenticator { Id = "primary" });
    ```

=== "Rust"

    ```rust
    use enserva_rust_client_example::EnservaUdpClient;

    let mut client = EnservaUdpClient::connect(
        "127.0.0.1:9000",
        "rust-client",
        "dev-token",
    )?;
    client.send_keep_alive()?;
    ```

Only one authentication object can be registered at a time. If an authentication object exists, unauthenticated UDP clients cannot receive snapshots or send normal object requests.

## Hiding Server-Only Objects

Implement `SnapshotVisible` when an object should exist in the runtime but not be sent to clients:

=== "GoLang"

    ```go
    func (auth *Authenticator) SnapshotVisible() bool {
    	return false
    }
    ```

=== "C#"

    ```csharp
    public bool SnapshotVisible() => false;
    ```

=== "Rust"

    ```rust
    fn snapshot_visible(&self) -> bool {
        false
    }
    ```

This is useful for authentication handlers and other server-only coordination objects.

## Included Example Package

The repository's `Enserva/netObjects` package includes:

| Example                                  | Demonstrates                                                       |
| ---------------------------------------- | ------------------------------------------------------------------ |
| `Player`                                 | An object with request, scene-switch, tick, and full-tick hooks.   |
| `Building`                               | An object with init, request, and full-tick hooks.                 |
| `PlayerAuthenticator`                    | A hidden authentication object that creates a server-owned player. |
| `Register(server *network.Server) error` | One way to group object/factory registration for an app package.   |

Read these files as implementation examples only. The object names, actions, payload fields, and gameplay behavior are not part of the Enserva core API.
