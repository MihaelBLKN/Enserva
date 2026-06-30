# Interest Management

Interest management lets Enserva send each player a smaller snapshot by including only spatial objects that are relevant to that player.

Without interest management, every authenticated client receives the same visible snapshot. With interest management enabled, the UDP server asks the runtime for a snapshot for each client, and the runtime uses a spatial hash to find registered spatial objects near that client's player object.

## How It Works

Interest management is opt-in per object:

- A player object registers as `network.InterestPlayer`.
- A game object registers as `network.InterestGameObject`.
- The player registration defines the interest radius.
- Each registration tells Enserva which snapshot fields represent position.
- Objects that do not opt in stay globally visible, unless they are hidden with `SnapshotVisible() false`.

The feature uses object snapshots for position extraction. That means the field names you provide should match either exported Go field names or JSON tag names from the value returned by `Snapshot()`.

## Register Interest In `OnInit`

`OnInit` runs when an object is registered with the runtime. It is the intended place to enable interest management for that object.

```go
func (player *Player) OnInit(ctx network.InitContext) {
	ctx.Runtime().Features().EnableInterestManagement(
		network.PlayerInterest(player, "x", "y", "z", 750),
	)
}
```

For non-player objects:

```go
func (building *Building) OnInit(ctx network.InitContext) {
	ctx.Runtime().Features().EnableInterestManagement(
		network.GameObjectInterest(building, "x", "y", "z"),
	)
}
```

The helper functions fill in the object's `ObjectType()` and `ObjectID()` for you, which keeps the setup to one line.

## 2D Objects

For 2D games, use the `2D` helpers and omit the Z field:

```go
func (player *Player) OnInit(ctx network.InitContext) {
	ctx.Runtime().Features().EnableInterestManagement(
		network.PlayerInterest2D(player, "x", "y", 500),
	)
}

func (tree *Tree) OnInit(ctx network.InitContext) {
	ctx.Runtime().Features().EnableInterestManagement(
		network.GameObjectInterest2D(tree, "x", "y"),
	)
}
```

Spatial hash lookups use X/Y cells, then exact distance checks use X/Y only for 2D registrations and X/Y/Z for 3D registrations.

## Field Names

Position fields can be Go struct field names:

```go
type PlayerSnapshot struct {
	X float64
	Y float64
	Z float64
}
```

Or JSON tag names:

```go
type PlayerSnapshot struct {
	PositionX float64 `json:"x"`
	PositionY float64 `json:"y"`
	PositionZ float64 `json:"z,omitempty"`
}
```

With the tagged version, use:

```go
network.PlayerInterest(player, "x", "y", "z", 750)
```

Map snapshots with string keys are also supported when their values are numeric or numeric strings.

## Snapshot Filtering

When UDP snapshots are broadcast:

1. The server collects authenticated snapshot clients.
2. For each client, the runtime finds the registered `InterestPlayer` whose object ID matches the client ID.
3. The runtime extracts that player's position from its snapshot.
4. Registered interest objects are placed into a spatial hash.
5. Nearby spatial hash cells are queried using the player's radius.
6. Far registered objects are filtered out.
7. Unregistered object types remain visible by default.
8. Objects with `SnapshotVisible() false` are always excluded.

This keeps interest management incremental. You can opt in only the object types that are expensive or noisy, while global state such as match timers can continue appearing in every snapshot.

## Manual Configuration

If you do not want to use the helper functions, you can pass `InterestManagementConfig` directly:

```go
ctx.Runtime().Features().EnableInterestManagement(network.InterestManagementConfig{
	SubjectType: network.InterestPlayer,
	ObjectType:  player.ObjectType(),
	ObjectID:    player.ObjectID(),
	XField:      "x",
	YField:      "y",
	ZField:      "z",
	Radius:      750,
	IncludeSelf: true,
})
```

Game objects use `network.InterestGameObject` and usually do not need a radius:

```go
ctx.Runtime().Features().EnableInterestManagement(network.InterestManagementConfig{
	SubjectType: network.InterestGameObject,
	ObjectType:  pickup.ObjectType(),
	ObjectID:    pickup.ObjectID(),
	XField:      "x",
	YField:      "y",
	ZField:      "z",
})
```

## Spatial Hash Behavior

The spatial hash is rebuilt from the current snapshot data when a client snapshot is generated. It acts as a broad phase, so objects in nearby cells are still checked against the configured radius before they are included. This keeps circular and spherical interest boundaries exact while avoiding a full radius comparison against every registered spatial object.
