# Examples

These examples are derived from the README, sample objects, and tests in the repository.

## Register the Sample Objects

=== "GoLang"

    ```go
    package main

    import (
    	"log"

    	netobjects "Enserva/netObjects"
    	"Enserva/network"
    )

    func main() {
    	server := network.NewServer(network.DefaultConfig())

    	if err := netobjects.Register(server); err != nil {
    		log.Fatal(err)
    	}

    	log.Fatal(server.ListenAndServe())
    }
    ```

=== "C#"

    ```csharp
    // C# clients connect to the Go Enserva host over UDP.
    // Keep the authoritative server in Go, then start your C# client/tool separately.
    using var client = new UdpClient();
    await client.SendAsync(Array.Empty<byte>(), 0, "127.0.0.1", 9000);
    ```

=== "Rust"

    ```rust
    use std::net::UdpSocket;

    let socket = UdpSocket::bind("0.0.0.0:0")?;
    socket.send_to(&[], "127.0.0.1:9000")?;
    ```

`netobjects.Register` binds the sample authenticator and factories for `player` and `building`.

## Create Server-Owned Objects

Factories are called only by server code:

=== "GoLang"

    ```go
    server := network.NewServer(network.DefaultConfig())

    if err := server.RegisterFactory("building", network.ObjectFactoryFunc(netobjects.BuildingFactory)); err != nil {
    	return err
    }

    building, err := server.CreateObject("building", "building-1")
    if err != nil {
    	return err
    }

    _ = building
    ```

=== "C#"

    ```csharp
    // Conceptual C# host API shape if you wrap Enserva from another runtime.
    var config = EnservaConfig.Default();
    var server = new EnservaServer(config);

    server.RegisterFactory("player", PlayerFactory.Create);
    server.RegisterAuthenticationObject(new PlayerAuthenticator("default"));

    await server.ListenAndServeAsync();
    ```

=== "Rust"

    ```rust
    let config = EnservaConfig::default();
    let mut server = EnservaServer::new(config);

    server.register_factory("building", building_factory)?;
    let building = server.create_object("building", "building-1")?;

    let _ = building;
    ```

!!! warning
A client request to a missing `building-1` returns `ErrObjectNotFound`. It does not call `BuildingFactory`.

## Write a Custom Object

=== "GoLang"

    ```go
    type Door struct {
    	ID     string `json:"id"`
    	Open   bool   `json:"open"`
    	Uses   uint64 `json:"uses"`
    	Client string `json:"client,omitempty"`
    }

    func (door *Door) ObjectType() string { return "door" }
    func (door *Door) ObjectID() string   { return door.ID }
    func (door *Door) Snapshot() any      { return *door }

    func (door *Door) OnRequest(ctx network.RequestContext) error {
    	switch ctx.Request.Action {
    	case "open":
    		door.Open = true
    	case "close":
    		door.Open = false
    	default:
    		return nil
    	}

    	door.Uses++
    	door.Client = ctx.ClientID
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
    door := &Door{ID: "door-1"}
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

Send a request with the preferred wire protocol. For generic object routing, the built-in `ObjectRequest` message keeps the UDP framing binary while adapting into the normal object request path:

=== "GoLang"

    ```go
    message, err := network.EncodeClientMessage(network.ObjectRequest{
    	ObjectType: "door",
    	ObjectID:   "door-1",
    	Action:     "open",
    })
    if err != nil {
    	return err
    }

    packet, err := network.EncodePacket(10, []network.WireMessage{message})
    if err != nil {
    	return err
    }

    _, err = conn.Write(packet)
    return err
    ```

=== "C#"

    ```csharp
    var message = EnservaWire.EncodeClientMessage(new ObjectRequest
    {
        ObjectType = "door",
        ObjectId = "door-1",
        Action = "open",
    });

    byte[] packet = EnservaWire.EncodePacket(sequence: 10, messages: [message]);
    await udp.SendAsync(packet, packet.Length, "127.0.0.1", 9000);
    ```

=== "Rust"

    ```rust
    use enserva_rust_client_example::{DeliveryClass, EnservaUdpClient};

    let mut client = EnservaUdpClient::connect(
        "127.0.0.1:9000",
        "rust-client",
        "dev-token",
    )?;

    client.send_object_request(
        "door",
        "door-1",
        "open",
        &[],
        DeliveryClass::Unreliable,
    )?;
    ```

For game-specific hot paths, register a typed message in the game range as shown in the [Wire Protocol](api/wire-protocol.md) guide.

The legacy JSON equivalent is still supported:

```json
{
  "seq": 10,
  "objectType": "door",
  "objectId": "door-1",
  "action": "open"
}
```

## Authenticate with a Domain Object

The tests show the intended authentication shape: an object decodes credentials, validates them, and returns the ID that should represent the client.

=== "GoLang"

    ```go
    type Authenticator struct {
    	ID string
    }

    func (auth *Authenticator) ObjectType() string { return "auth" }
    func (auth *Authenticator) ObjectID() string   { return auth.ID }
    func (auth *Authenticator) Snapshot() any      { return nil }
    func (auth *Authenticator) SnapshotVisible() bool {
    	return false
    }

    func (auth *Authenticator) OnAuthenticationAttempt(ctx network.AuthenticationContext) (string, error) {
    	var payload struct {
    		Token string `json:"token"`
    	}
    	if err := ctx.Decode(&payload); err != nil {
    		return "", err
    	}
    	if payload.Token != "valid-token" {
    		return "", errors.New("invalid token")
    	}

    	return "player-a", nil
    }
    ```

=== "C#"

    ```csharp
    public sealed class Authenticator : IAuthenticationHandler, IEnservaObject
    {
        public string Id { get; init; } = "";
        public string ObjectType => "auth";
        public string ObjectId => Id;
        public object? Snapshot() => null;
        public bool SnapshotVisible() => false;

        public string OnAuthenticationAttempt(AuthenticationContext ctx)
        {
            var payload = ctx.Decode<AuthPayload>();
            if (payload.Token != "valid-token")
                throw new InvalidOperationException("invalid token");

            return "player-a";
        }
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
    if err := server.RegisterAuthenticationObject(&Authenticator{ID: "primary"}); err != nil {
    	return err
    }
    ```

=== "C#"

    ```csharp
    server.RegisterAuthenticationObject(new Authenticator { Id = "primary" });
    ```

=== "Rust"

    ```rust
    server.register_authentication_object(Authenticator {
        id: "primary".into(),
    })?;
    ```

## Test Request Routing Without UDP

You can test object behavior directly through `Runtime.HandleRequest`:

=== "GoLang"

    ```go
    runtime := network.NewRuntime(network.Config{})
    player := netobjects.NewPlayer("player-1")
    player.AssignClient("player-1")

    if err := runtime.RegisterObject(player); err != nil {
    	t.Fatal(err)
    }

    err := runtime.HandleRequest(network.RequestContext{
    	ClientID: "player-1",
    	Request: network.RequestMessage{
    		ObjectType: "player",
    		ObjectID:   "player-1",
    		Action:     "input",
    		Data:       json.RawMessage(`{"x":1,"y":0}`),
    	},
    })
    if err != nil {
    	t.Fatal(err)
    }
    ```

=== "C#"

    ```csharp
    var runtime = new EnservaRuntime(new EnservaConfig());
    ```

=== "Rust"

    ```rust
    let mut runtime = EnservaRuntime::new(EnservaConfig::default());
    ```

This pattern avoids UDP timing and lets tests assert object state directly.

## Use Direct Responses

Objects can send an immediate response when the transport supplies a `ResponseWriter`:

=== "GoLang"

    ```go
    func (door *Door) OnRequest(ctx network.RequestContext) error {
    	door.Open = true

    	return ctx.Respond(network.ResponseMessage{
    		Type:     "door",
    		Sequence: ctx.Request.Sequence,
    		OK:       true,
    		Data: map[string]any{
    			"open": door.Open,
    		},
    	})
    }
    ```

=== "C#"

    ```csharp
    public Task OnRequest(RequestContext ctx)
    {
        Open = true;

        return ctx.RespondAsync(new ResponseMessage
        {
            Type = "door",
            Sequence = ctx.Request.Sequence,
            Ok = true,
            Data = new Dictionary<string, object> { ["open"] = Open },
        });
    }
    ```

=== "Rust"

    ```rust
    fn on_request(&mut self, ctx: &mut RequestContext) -> Result<(), ResponseError> {
        self.open = true;

        ctx.respond(ResponseMessage {
            message_type: "door".into(),
            sequence: ctx.sequence,
            ok: true,
        })
    }
    ```

!!! note
Direct responses are optional. `ctx.Respond` returns `ErrResponsesUnsupported` if no response writer is available.
