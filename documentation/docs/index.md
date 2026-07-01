---
hide:
  - navigation
  - toc
---

<div class="ev-hero" markdown>
<div class="ev-hero-inner" markdown>

<span class="ev-eyebrow"><span class="dot"></span> Early development build v1.0</span>

# A Go runtime for authoritative multiplayer servers.

<p class="ev-sub">Enserva is an early game server runtime written in Go. It combines an authoritative tick loop, UDP transport, server-owned objects, scene-aware snapshots, interest management, and a local debug UI so multiplayer games can keep simulation and replication under server control while the API is still evolving.</p>

<div class="ev-actions">
  <a href="installation/" class="ev-btn ev-btn-primary">Get Started -></a>
  <a href="https://github.com/MihaelBLKN/Enserva" class="ev-btn ev-btn-secondary">View on GitHub</a>
</div>

</div>
</div>

<div class="ev-stats" markdown>
<div markdown>
<div class="ev-stat-num">UDP</div>
<div class="ev-stat-label">Low-level transport</div>
</div>
<div markdown>
<div class="ev-stat-num">Tick</div>
<div class="ev-stat-label">Server simulation</div>
</div>
<div markdown>
<div class="ev-stat-num">Scenes</div>
<div class="ev-stat-label">Server-owned worlds</div>
</div>
<div markdown>
<div class="ev-stat-num">Go</div>
<div class="ev-stat-label">Dependency-light runtime</div>
</div>
</div>

Enserva is built for authoritative multiplayer servers where the backend owns object lifetime, routing, visibility, scenes, and snapshot delivery. The networking layer is part of the server rather than the whole product: game code plugs into a Go runtime that already knows how to tick, receive client requests, filter snapshots, and expose development state.

!!! warning "Early project notice"
Do not use Enserva for real-world production game servers unless you are fully willing and able to absorb large breaking changes.

    Enserva is still in a very early stage. You should expect:

    - Bugs in core networking, runtime, and replication features.
    - Updates that remove or redesign existing APIs and behavior.
    - Changes in protocol details, especially around wire packets, snapshots, authentication, and object routing.
    - Changing advice on coding conventions, message schemas, and how to structure server projects.

    This is not a bad thing. Moving quickly at this stage lets Enserva abandon weak ideas, simplify counterproductive abstractions, and find stronger foundations for authoritative multiplayer servers.

    Do not be discouraged from experimenting with Enserva. It is a good time to follow development, try the runtime in prototypes, and help shape the API. More stable, long-term versions will come once the project has had time to settle.

## Features

<div class="ev-grid" markdown>

<div class="ev-card" markdown>
<div class="ev-card-icon">01</div>
#### Authoritative game state
The server owns object lifetime, request routing, scene membership, and replication decisions.
</div>

<div class="ev-card" markdown>
<div class="ev-card-icon">02</div>
#### Go simulation loop
Per-tick and full-second hooks give game systems predictable timing inside a Go server process.
</div>

<div class="ev-card" markdown>
<div class="ev-card-icon">03</div>
#### UDP transport
Binary packets are the primary client path, with legacy JSON compatibility kept for tools and debugging.
</div>

<div class="ev-card" markdown>
<div class="ev-card-icon">04</div>
#### Snapshot replication
The runtime builds per-client snapshots and keeps replicated state scoped to what each client should see.
</div>

<div class="ev-card" markdown>
<div class="ev-card-icon">05</div>
#### Interest management
Per-client interest filtering trims snapshot payloads to nearby or relevant objects.
</div>

<div class="ev-card" markdown>
<div class="ev-card-icon">06</div>
#### Scene management
Server-owned scene membership supports rooms, maps, shards, scene switches, and scene-filtered snapshots.
</div>

<div class="ev-card" markdown>
<div class="ev-card-icon">07</div>
#### Object-based auth
An optional authentication flow lives inside the object model instead of a separate side channel, with one registered authority for mapping connections to player identities.
</div>

<div class="ev-card" markdown>
<div class="ev-card-icon">08</div>
#### Debug UI
A browser debug UI exposes runtime, transport, feature, object, and scene state for local development.
</div>

</div>
