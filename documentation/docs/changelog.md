# Changelog

No release tags have been added yet.

## Unreleased

- Added authoritative runtime object registration, request routing, authentication hooks, snapshot generation, and server-side factories.
- Added the UDP transport with sequencing, duplicate request rejection, authentication gating, client timeouts, direct responses, and snapshot broadcasts.
- Added binary wire packet support with message registration, built-in protocol and engine messages, acknowledgement fields, and typed payload decoding.
- Added interest management with 2D and 3D helpers, snapshot field extraction, and spatial hash filtering.
- Added the browser debug UI and `/debug/state` endpoint for config, runtime, feature, UDP, and object diagnostics.
- Added sample `Player`, `Building`, and `PlayerAuthenticator` objects plus tests for runtime, authentication, interest management, debug state, and wire behavior.
