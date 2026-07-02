# Enserva Rust Client Example

This example is a dependency-free Rust client for Enserva's binary UDP wire protocol.
It covers:

- `protocol.hello` authentication and capability negotiation.
- `protocol.ping` / `protocol.pong`.
- `engine.player_input`.
- `engine.client_input`.
- `engine.object_request`.
- Custom game messages in the `0x1000..=0xffff` range.
- Packet acknowledgements and basic reliable ordered/unordered retry.
- Decoding welcome, error, disconnect, full snapshot, and delta snapshot messages.

Run the example while an Enserva UDP server is listening on `127.0.0.1:9000`:

```bash
cargo run -- 127.0.0.1:9000 rust-client dev-token
```

Run the syntax and behavior checks:

```bash
cargo test
```

The crate intentionally uses only Rust's standard library so it can be copied into a
game client or engine integration without pulling in an async runtime.
