package tests

import (
	"Enserva/network"
	"encoding/json"
	"testing"
)

type sceneSwitchTestObject struct {
	id           string
	locked       bool
	currentScene network.SceneID
	targetScene  network.SceneID
}

func (object *sceneSwitchTestObject) ObjectType() string {
	return "player"
}

func (object *sceneSwitchTestObject) ObjectID() string {
	return object.id
}

func (object *sceneSwitchTestObject) Snapshot() any {
	return map[string]any{"id": object.id}
}

func (object *sceneSwitchTestObject) OnSceneSwitchRequest(ctx network.SceneSwitchContext) (network.SceneSwitchDecision, error) {
	object.currentScene = ctx.CurrentScene
	object.targetScene = ctx.TargetScene
	if object.locked {
		return network.SceneSwitchDenied("locked"), nil
	}

	return network.SceneSwitchAllowed(), nil
}

// TestSnapshotForClientFiltersObjectsByScene verifies server-owned scene visibility.
func TestSnapshotForClientFiltersObjectsByScene(t *testing.T) {
	runtime := network.NewRuntime(network.Config{})
	mustRegisterInterestObject(t, runtime, &interestTestObject{objectType: "player", id: "player-1"})
	mustRegisterInterestObject(t, runtime, &interestTestObject{objectType: "building", id: "map-1-building"})
	mustRegisterInterestObject(t, runtime, &interestTestObject{objectType: "building", id: "map-2-building"})
	mustRegisterInterestObject(t, runtime, &interestTestObject{objectType: "match", id: "global"})
	mustRegisterInterestObject(t, runtime, &interestTestObject{objectType: "system", id: "unassigned"})

	features := runtime.Features()
	features.SetClientScene("client-1", "map-1")
	features.SetObjectScene("player", "player-1", "map-1")
	features.SetObjectScene("building", "map-1-building", "map-1")
	features.SetObjectScene("building", "map-2-building", "map-2")
	features.SetObjectGlobal("match", "global")

	snapshot := runtime.SnapshotForClient("client-1")
	if !hasSnapshotObject(snapshot, "player", "player-1") {
		t.Fatalf("expected player in client scene snapshot: %#v", snapshot)
	}
	if !hasSnapshotObject(snapshot, "building", "map-1-building") {
		t.Fatalf("expected same-scene building in snapshot: %#v", snapshot)
	}
	if hasSnapshotObject(snapshot, "building", "map-2-building") {
		t.Fatalf("expected other-scene building to be filtered: %#v", snapshot)
	}
	if !hasSnapshotObject(snapshot, "match", "global") {
		t.Fatalf("expected global object in snapshot: %#v", snapshot)
	}
	if !hasSnapshotObject(snapshot, "system", "unassigned") {
		t.Fatalf("expected unassigned object to remain globally visible: %#v", snapshot)
	}

	unfiltered := runtime.SnapshotForClient("client-without-scene")
	if !hasSnapshotObject(unfiltered, "building", "map-2-building") {
		t.Fatalf("expected clients without a scene to receive unfiltered snapshots: %#v", unfiltered)
	}
}

// TestSnapshotForClientCombinesSceneAndInterestFilters verifies scene filtering happens before interest output.
func TestSnapshotForClientCombinesSceneAndInterestFilters(t *testing.T) {
	runtime := network.NewRuntime(network.Config{})
	mustRegisterInterestObject(t, runtime, &interestTestObject{
		objectType:  "player",
		id:          "player-1",
		subject:     network.InterestPlayer,
		radius:      10,
		includeSelf: true,
	})
	mustRegisterInterestObject(t, runtime, &interestTestObject{
		objectType: "building",
		id:         "same-scene-near",
		subject:    network.InterestGameObject,
		x:          6,
		y:          8,
	})
	mustRegisterInterestObject(t, runtime, &interestTestObject{
		objectType: "building",
		id:         "same-scene-far",
		subject:    network.InterestGameObject,
		x:          11,
	})
	mustRegisterInterestObject(t, runtime, &interestTestObject{
		objectType: "building",
		id:         "other-scene-near",
		subject:    network.InterestGameObject,
		x:          1,
	})

	features := runtime.Features()
	features.SetClientScene("player-1", "map-1")
	features.SetObjectScene("player", "player-1", "map-1")
	features.SetObjectScene("building", "same-scene-near", "map-1")
	features.SetObjectScene("building", "same-scene-far", "map-1")
	features.SetObjectScene("building", "other-scene-near", "map-2")

	snapshot := runtime.SnapshotForClient("player-1")
	if !hasSnapshotObject(snapshot, "building", "same-scene-near") {
		t.Fatalf("expected nearby same-scene object in snapshot: %#v", snapshot)
	}
	if hasSnapshotObject(snapshot, "building", "same-scene-far") {
		t.Fatalf("expected far same-scene object to be filtered by interest: %#v", snapshot)
	}
	if hasSnapshotObject(snapshot, "building", "other-scene-near") {
		t.Fatalf("expected nearby other-scene object to be filtered by scene: %#v", snapshot)
	}
}

// TestRequestSceneSwitchMutatesStateOnlyWhenAllowed verifies scene switches are server-authorized.
func TestRequestSceneSwitchMutatesStateOnlyWhenAllowed(t *testing.T) {
	runtime := network.NewRuntime(network.Config{})
	object := &sceneSwitchTestObject{id: "player-1"}
	mustRegisterInterestObject(t, runtime, object)

	features := runtime.Features()
	features.SetObjectScene("player", "player-1", "map-1")
	features.SetClientScene("client-1", "map-1")

	payload, err := json.Marshal(network.SceneSwitchRequest{TargetScene: "map-2"})
	if err != nil {
		t.Fatalf("marshal scene request: %v", err)
	}

	var allowedResponse network.SceneSwitchResponse
	err = runtime.HandleRequest(network.RequestContext{
		ClientID: "client-1",
		Request: network.RequestMessage{
			ObjectType: "player",
			ObjectID:   "player-1",
			Action:     "scene.switch",
			Data:       payload,
		},
		Response: network.ResponseWriterFunc(func(message any) error {
			response, ok := message.(network.SceneSwitchResponse)
			if !ok {
				t.Fatalf("expected scene switch response, got %#v", message)
			}
			allowedResponse = response
			return nil
		}),
	})
	if err != nil {
		t.Fatalf("handle allowed scene switch: %v", err)
	}
	if !allowedResponse.OK || allowedResponse.Scene != "map-2" || allowedResponse.PreviousScene != "map-1" {
		t.Fatalf("expected allowed map-2 response from map-1, got %#v", allowedResponse)
	}
	if object.currentScene != "map-1" || object.targetScene != "map-2" {
		t.Fatalf("unexpected switch context current=%q target=%q", object.currentScene, object.targetScene)
	}
	if scene, ok := features.ObjectScene("player", "player-1"); !ok || scene != "map-2" {
		t.Fatalf("expected object scene map-2, got %q ok=%v", scene, ok)
	}
	if scene, ok := features.ClientScene("client-1"); !ok || scene != "map-2" {
		t.Fatalf("expected client scene map-2, got %q ok=%v", scene, ok)
	}

	object.locked = true
	payload, err = json.Marshal(network.SceneSwitchRequest{TargetScene: "map-3"})
	if err != nil {
		t.Fatalf("marshal denied scene request: %v", err)
	}

	var deniedResponse network.SceneSwitchResponse
	err = runtime.HandleRequest(network.RequestContext{
		ClientID: "client-1",
		Request: network.RequestMessage{
			ObjectType: "player",
			ObjectID:   "player-1",
			Action:     "scene.switch",
			Data:       payload,
		},
		Response: network.ResponseWriterFunc(func(message any) error {
			response, ok := message.(network.SceneSwitchResponse)
			if !ok {
				t.Fatalf("expected scene switch response, got %#v", message)
			}
			deniedResponse = response
			return nil
		}),
	})
	if err != nil {
		t.Fatalf("handle denied scene switch: %v", err)
	}
	if deniedResponse.OK || deniedResponse.Reason != "locked" || deniedResponse.PreviousScene != "map-2" {
		t.Fatalf("expected locked denial response from map-2, got %#v", deniedResponse)
	}
	if scene, ok := features.ObjectScene("player", "player-1"); !ok || scene != "map-2" {
		t.Fatalf("expected denied switch to preserve object scene map-2, got %q ok=%v", scene, ok)
	}
	if scene, ok := features.ClientScene("client-1"); !ok || scene != "map-2" {
		t.Fatalf("expected denied switch to preserve client scene map-2, got %q ok=%v", scene, ok)
	}
}
