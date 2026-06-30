# Changelog

No release tags have been added yet.

## Unreleased

- Added authoritative runtime object registration, request routing, authentication hooks, snapshot generation, and server-side factories.
- Added the UDP transport with sequencing, duplicate request rejection, authentication gating, client timeouts, direct responses, snapshot broadcasts, and binary wire packet support.
- Added a binary wire protocol registry with built-in hello, welcome, ping, pong, error, disconnect, object request, player input, world snapshot, and entity delta messages.
- Added interest management with 2D and 3D helpers, snapshot field extraction, spatial hash filtering, and scene-aware snapshot composition.
- Added scene management with `SceneID`, client/object scene assignment, global scene objects, scene-filtered snapshots, debug visibility, and cleanup when objects are removed.
- Added server-authorized scene switching with `SceneSwitchHandler`, scene switch request/response types, decisions for allow/deny/redirect, direct `scene.switch` routing to `OnSceneSwitchRequest`, and immediate scene switch responses.
- Updated the sample player/authentication flow to assign default scenes, keep players and clients in sync, and enforce player ownership during scene switches.
- Added the browser debug UI and `/debug/state` endpoint for config, runtime, feature, UDP, scene, interest, and object diagnostics.
- Added a scene-management panel to the browser debug UI and switched the debug interface to a full dark-mode palette.
- Expanded tests for runtime authority, authentication, player ownership, wire behavior, interest management, scene filtering, scene switching, and debug state.
- Reworked the documentation site with MkDocs Material navigation, a custom dark theme, browser debug docs, wire protocol docs, feature guides, API reference pages, and GoLang/C# tabbed code snippets.
- Moved the Scenes guide under the Features navigation category and made sidebar category headings more visually prominent.
