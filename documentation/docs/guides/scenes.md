# Scenes

Scenes let the server divide connected clients and objects into server-owned worlds, maps, rooms, shards, or phases. They are a snapshot visibility feature first: the runtime uses scene membership to decide which objects a client can see.

!!! note
    Scenes do not create objects, load maps, or move clients by themselves. Your game code owns those rules. Enserva stores scene membership, applies snapshot filtering, and provides a server-authorized scene switch flow.

## How Scene Visibility Works

Scene state lives on `Runtime.Features()`:

=== "GoLang"

    ```go
    features := runtime.Features()
    features.SetClientScene("player-1", "arena-a")
    features.SetObjectScene("player", "player-1", "arena-a")
    features.SetObjectScene("building", "tower-a", "arena-a")
    features.SetObjectScene("building", "tower-b", "arena-b")
    ```

=== "C#"

    ```csharp
    var features = server.Runtime.Features;
    features.SetClientScene("player-1", "arena-a");
    features.SetObjectScene("player", "player-1", "arena-a");
    features.SetObjectScene("building", "tower-a", "arena-a");
    features.SetObjectScene("building", "tower-b", "arena-b");
    ```

When `Runtime.SnapshotForClient("player-1")` runs:

1. The runtime finds the client's current scene.
2. Objects assigned to the same scene are included.
3. Objects assigned to a different scene are filtered out.
4. Objects without an explicit scene remain visible everywhere.
5. Objects marked with `SceneGlobal` remain visible everywhere.
6. Interest management runs after scene filtering when both features are enabled.

Clients without a scene receive unfiltered snapshots. Assign client scenes before relying on scene isolation.

## Assigning Scenes

Use the feature helpers to assign and inspect scene state:

=== "GoLang"

    ```go
    features := server.Runtime().Features()

    features.SetClientScene("player-1", "arena-a")
    features.SetObjectScene("player", "player-1", "arena-a")
    features.SetObjectScene("building", "tower-a", "arena-a")
    features.SetObjectGlobal("match", "scoreboard")
    ```

=== "C#"

    ```csharp
    var features = server.Runtime.Features;
    features.SetClientScene("player-1", "arena-a");
    features.SetObjectScene("player", "player-1", "arena-a");
    features.SetObjectScene("building", "tower-a", "arena-a");
    features.SetObjectScene("building", "tower-b", "arena-b");
    ```

Useful methods:

| Method | Purpose |
| --- | --- |
| `SetClientScene(clientID, sceneID)` | Assigns a client to the scene used for snapshot filtering. |
| `SetObjectScene(objectType, objectID, sceneID)` | Assigns an object to a scene. |
| `SetObjectSceneForObject(object, sceneID)` | Assigns a registered object using its `ObjectType()` and `ObjectID()`. |
| `SetObjectGlobal(objectType, objectID)` | Marks an object as visible to clients in every scene. |
| `ClearClientScene(clientID)` | Removes a client's explicit scene assignment. |
| `ClearObjectScene(objectType, objectID)` | Removes an object's explicit scene assignment. |
| `ClientScene(clientID)` | Reads a client's current scene. |
| `ObjectScene(objectType, objectID)` | Reads an object's current scene. |

Scene IDs are normalized the same way object keys are normalized: surrounding whitespace is trimmed and empty IDs are rejected.

## Global and Unassigned Objects

There are two ways for an object to appear in every scene:

=== "GoLang"

    ```go
    features.SetObjectGlobal("match", "scoreboard")
    ```

=== "C#"

    ```csharp
    features.SetObjectGlobal("match", "scoreboard");
    ```

or leave the object unassigned:

=== "GoLang"

    ```go
    features.ClearObjectScene("system", "clock")
    ```

=== "C#"

    ```csharp
    features.ClearObjectScene("system", "clock");
    ```

Use `SceneGlobal` when global visibility is intentional and should survive scene bookkeeping. Leave objects unassigned for simple server objects that do not need scene membership yet.

## Scene Switch Requests

Scene switches are server-authorized. Standard scene switch requests are routed directly to the target object's `SceneSwitchHandler`, so the object does not need to implement `OnRequest` just to change scenes:

=== "GoLang"

    ```go
    func (player *Player) OnSceneSwitchRequest(ctx network.SceneSwitchContext) (network.SceneSwitchDecision, error) {
    	if player.OwnerClientID != "" && ctx.ClientID != player.OwnerClientID {
    		return network.SceneSwitchDecision{}, ErrUnauthorizedPlayerClient
    	}
    	if !player.CanEnter(ctx.TargetScene) {
    		return network.SceneSwitchDenied("locked"), nil
    	}

    	return network.SceneSwitchAllowed(), nil
    }
    ```

=== "C#"

    ```csharp
    public SceneSwitchDecision OnSceneSwitchRequest(SceneSwitchContext ctx)
    {
        if (!CanEnter(ctx.TargetScene))
            return SceneSwitchDecision.Denied("locked");

        return SceneSwitchDecision.Allowed();
    }
    ```

When the decision is allowed, Enserva updates the object's scene and, when `ClientID` is present, the client's scene. Denied switches do not mutate scene state.

Legacy JSON clients can send a `RequestMessage` with `Action: "scene.switch"` and a `SceneSwitchRequest` in `Data`. The runtime also recognizes `scene`, `switchScene`, and `switch_scene` for compatibility, and responds with `SceneSwitchResponse` when the transport supports immediate responses.

## Redirecting or Clearing Client Objects

`SceneSwitchAllowed()` accepts the requested scene and sets `ClearClientObjects` to `true`. This tells clients that they should discard scene-local state before accepting the new snapshot stream.

Use `SceneSwitchAllowedTo(sceneID)` to redirect a request:

=== "GoLang"

    ```go
    func (player *Player) OnSceneSwitchRequest(ctx network.SceneSwitchContext) (network.SceneSwitchDecision, error) {
    	if ctx.TargetScene == "ranked" && !player.RankedUnlocked {
    		return network.SceneSwitchAllowedTo("lobby"), nil
    	}

    	return network.SceneSwitchAllowed(), nil
    }
    ```

=== "C#"

    ```csharp
    public SceneSwitchDecision OnSceneSwitchRequest(SceneSwitchContext ctx)
    {
        if (ctx.TargetScene == "ranked" && !RankedUnlocked)
            return SceneSwitchDecision.AllowedTo("lobby");

        return SceneSwitchDecision.Allowed();
    }
    ```

Use `SceneSwitchDenied(reason)` when the current scene should be preserved.

## Scenes and Wire Packets

Scenes are transport-agnostic. The preferred client path is still the binary wire protocol, usually through a custom game message in the `0x1000-0xffff` range or a built-in compatibility request with the `scene.switch` action.

The standard scene switch payload is:

=== "GoLang"

    ```go
    type SceneSwitchRequest struct {
    	TargetScene network.SceneID `json:"targetScene"`
    }
    ```

=== "C#"

    ```csharp
    public sealed class SceneSwitchRequest
    {
        public string TargetScene { get; set; } = "";
    }
    ```

Legacy JSON clients can send the same payload in `RequestMessage.Data`. Wire clients should encode an equivalent typed payload or use a registered scene-switch message for their game.

## Debugging Scenes

The debug state includes scene-management data when debug mode is enabled. Use it to inspect:

- Whether scene management is enabled.
- Client scene assignments.
- Object scene assignments.
- Object capabilities, including `sceneSwitch` for objects that implement `SceneSwitchHandler`.

Run the example host with:

```bash
go run . -debug
```

Then open `http://localhost:9100`.
