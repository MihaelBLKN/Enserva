package tests

import (
	"Enserva/network"
	"testing"
)

type interestTestObject struct {
	objectType  string
	id          string
	subject     network.InterestSubject
	x           float64
	y           float64
	z           float64
	radius      float64
	includeSelf bool
	hidden      bool
	xField      string
	yField      string
	zField      string
	tagged      bool
	initCalls   int
	initType    string
	initID      string
	initRuntime *network.Runtime
}

type interestXYZSnapshot struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	Z  float64 `json:"z,omitempty"`
}

type interestTaggedSnapshot struct {
	ID        string  `json:"id"`
	PositionX float64 `json:"positionX"`
	PositionY float64 `json:"positionY"`
	PositionZ float64 `json:"positionZ,omitempty"`
}

func (object *interestTestObject) ObjectType() string {
	return object.objectType
}

func (object *interestTestObject) ObjectID() string {
	return object.id
}

func (object *interestTestObject) Snapshot() any {
	if object.tagged {
		return interestTaggedSnapshot{
			ID:        object.id,
			PositionX: object.x,
			PositionY: object.y,
			PositionZ: object.z,
		}
	}

	return interestXYZSnapshot{
		ID: object.id,
		X:  object.x,
		Y:  object.y,
		Z:  object.z,
	}
}

func (object *interestTestObject) SnapshotVisible() bool {
	return !object.hidden
}

func (object *interestTestObject) OnInit(ctx network.InitContext) {
	object.initCalls++
	object.initType = ctx.ObjectType()
	object.initID = ctx.ObjectID()
	object.initRuntime = ctx.Runtime()

	if object.subject == "" || ctx.Runtime() == nil {
		return
	}

	xField := object.xField
	if xField == "" {
		xField = "x"
	}
	yField := object.yField
	if yField == "" {
		yField = "y"
	}

	ctx.Runtime().Features().EnableInterestManagement(network.InterestManagementConfig{
		SubjectType: object.subject,
		ObjectType:  object.ObjectType(),
		ObjectID:    object.ObjectID(),
		XField:      xField,
		YField:      yField,
		ZField:      object.zField,
		Radius:      object.radius,
		IncludeSelf: object.includeSelf,
	})
}

func TestRegisterObjectCallsOnInit(t *testing.T) {
	runtime := network.NewRuntime(network.Config{})
	object := &interestTestObject{objectType: "thing", id: "alpha"}
	mustRegisterInterestObject(t, runtime, object)

	if object.initCalls != 1 {
		t.Fatalf("expected one init call, got %d", object.initCalls)
	}
	if object.initType != "thing" || object.initID != "alpha" {
		t.Fatalf("expected init identity thing/alpha, got %s/%s", object.initType, object.initID)
	}
	if object.initRuntime != runtime {
		t.Fatalf("expected init context runtime to match registered runtime")
	}
}

func TestSnapshotForClientReturnsFullSnapshotWhenInterestDisabled(t *testing.T) {
	runtime := network.NewRuntime(network.Config{})
	mustRegisterInterestObject(t, runtime, &interestTestObject{objectType: "player", id: "player-1"})
	mustRegisterInterestObject(t, runtime, &interestTestObject{objectType: "building", id: "far", x: 1000})

	snapshot := runtime.SnapshotForClient("player-1")
	if !hasSnapshotObject(snapshot, "player", "player-1") {
		t.Fatalf("expected player in snapshot: %#v", snapshot)
	}
	if !hasSnapshotObject(snapshot, "building", "far") {
		t.Fatalf("expected full snapshot when interest is disabled: %#v", snapshot)
	}
}

func TestSnapshotForClientFiltersManagedObjectsByPlayerInterest(t *testing.T) {
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
		id:         "near",
		subject:    network.InterestGameObject,
		x:          6,
		y:          8,
	})
	mustRegisterInterestObject(t, runtime, &interestTestObject{
		objectType: "building",
		id:         "far",
		subject:    network.InterestGameObject,
		x:          11,
	})
	mustRegisterInterestObject(t, runtime, &interestTestObject{
		objectType: "building",
		id:         "hidden",
		subject:    network.InterestGameObject,
		x:          1,
		hidden:     true,
	})
	mustRegisterInterestObject(t, runtime, &interestTestObject{
		objectType: "unmanaged",
		id:         "global",
		x:          1000,
	})

	runtime.Advance()
	snapshot := runtime.SnapshotForClient("player-1")

	if !hasSnapshotObject(snapshot, "player", "player-1") {
		t.Fatalf("expected player to see itself: %#v", snapshot)
	}
	if !hasSnapshotObject(snapshot, "building", "near") {
		t.Fatalf("expected nearby managed object in snapshot: %#v", snapshot)
	}
	if hasSnapshotObject(snapshot, "building", "far") {
		t.Fatalf("expected far managed object to be filtered: %#v", snapshot)
	}
	if hasSnapshotObject(snapshot, "building", "hidden") {
		t.Fatalf("expected hidden object to stay hidden: %#v", snapshot)
	}
	if !hasSnapshotObject(snapshot, "unmanaged", "global") {
		t.Fatalf("expected unmanaged object to stay globally visible: %#v", snapshot)
	}
}

func TestSnapshotForClientSupportsJSONTagged3DFields(t *testing.T) {
	runtime := network.NewRuntime(network.Config{})
	mustRegisterInterestObject(t, runtime, &interestTestObject{
		objectType:  "player",
		id:          "player-1",
		subject:     network.InterestPlayer,
		radius:      5,
		includeSelf: true,
		xField:      "positionX",
		yField:      "positionY",
		zField:      "positionZ",
		tagged:      true,
	})
	mustRegisterInterestObject(t, runtime, &interestTestObject{
		objectType: "pickup",
		id:         "near",
		subject:    network.InterestGameObject,
		z:          5,
		xField:     "positionX",
		yField:     "positionY",
		zField:     "positionZ",
		tagged:     true,
	})
	mustRegisterInterestObject(t, runtime, &interestTestObject{
		objectType: "pickup",
		id:         "far",
		subject:    network.InterestGameObject,
		z:          6,
		xField:     "positionX",
		yField:     "positionY",
		zField:     "positionZ",
		tagged:     true,
	})

	runtime.Advance()
	snapshot := runtime.SnapshotForClient("player-1")

	if !hasSnapshotObject(snapshot, "pickup", "near") {
		t.Fatalf("expected nearby 3D object in snapshot: %#v", snapshot)
	}
	if hasSnapshotObject(snapshot, "pickup", "far") {
		t.Fatalf("expected far 3D object to be filtered: %#v", snapshot)
	}
}

func mustRegisterInterestObject(t *testing.T, runtime *network.Runtime, object network.Object) {
	t.Helper()

	if err := runtime.RegisterObject(object); err != nil {
		t.Fatalf("register object: %v", err)
	}
}

func hasSnapshotObject(snapshot network.SnapshotData, objectType, objectID string) bool {
	objectsByID := snapshot[objectType]
	if objectsByID == nil {
		return false
	}

	_, ok := objectsByID[objectID]
	return ok
}
