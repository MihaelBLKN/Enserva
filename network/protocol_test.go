package network

import (
	"encoding/json"
	"net"
	"testing"
	"time"
)

type testObject struct {
	id        string
	value     int
	ticks     int
	fullTicks int
	requests  int
	clientID  string
}

type testPayload struct {
	Value int `json:"value"`
}

type testSnapshot struct {
	ID        string `json:"id"`
	Value     int    `json:"value"`
	Ticks     int    `json:"ticks"`
	FullTicks int    `json:"fullTicks"`
	Requests  int    `json:"requests"`
	ClientID  string `json:"clientId"`
}

func (object *testObject) ObjectType() string {
	return "thing"
}

func (object *testObject) ObjectID() string {
	return object.id
}

func (object *testObject) Snapshot() any {
	return testSnapshot{
		ID:        object.id,
		Value:     object.value,
		Ticks:     object.ticks,
		FullTicks: object.fullTicks,
		Requests:  object.requests,
		ClientID:  object.clientID,
	}
}

func (object *testObject) OnTick(ctx TickContext) {
	object.ticks++
}

func (object *testObject) OnFullTick(ctx TickContext) {
	object.fullTicks++
}

func (object *testObject) OnRequest(ctx RequestContext) error {
	var payload testPayload
	if err := ctx.Decode(&payload); err != nil {
		return err
	}

	object.value = payload.Value
	object.requests++
	object.clientID = ctx.ClientID
	return nil
}

func TestRuntimeCreatesObjectsAndRunsHooks(t *testing.T) {
	runtime := NewRuntime(Config{TickRate: 2, SnapshotRate: 1})
	err := runtime.RegisterFactory("thing", ObjectFactoryFunc(func(ctx RequestContext) (Object, error) {
		return &testObject{id: ctx.Request.ObjectID}, nil
	}))
	if err != nil {
		t.Fatalf("register factory: %v", err)
	}

	err = runtime.HandleRequest(RequestContext{
		ClientID: "client-a",
		Request: RequestMessage{
			ObjectType: "thing",
			ObjectID:   "alpha",
			Data:       json.RawMessage(`{"value":42}`),
		},
	})
	if err != nil {
		t.Fatalf("handle request: %v", err)
	}

	runtime.Advance()
	runtime.Advance()

	snapshot := runtime.Snapshot()
	thing, ok := snapshot["thing"]["alpha"].(testSnapshot)
	if !ok {
		t.Fatalf("expected thing alpha snapshot, got %#v", snapshot)
	}

	if thing.Value != 42 {
		t.Fatalf("expected request value 42, got %d", thing.Value)
	}
	if thing.Ticks != 2 {
		t.Fatalf("expected two OnTick calls, got %d", thing.Ticks)
	}
	if thing.FullTicks != 1 {
		t.Fatalf("expected one OnFullTick call, got %d", thing.FullTicks)
	}
	if thing.Requests != 1 {
		t.Fatalf("expected one request, got %d", thing.Requests)
	}
	if thing.ClientID != "client-a" {
		t.Fatalf("expected client id to be forwarded, got %q", thing.ClientID)
	}
}

func TestRuntimeRejectsUnknownObjectsWithoutFactory(t *testing.T) {
	runtime := NewRuntime(Config{})

	err := runtime.HandleRequest(RequestContext{
		Request: RequestMessage{
			ObjectType: "thing",
			ObjectID:   "missing",
		},
	})
	if err == nil {
		t.Fatalf("expected unknown object request to fail")
	}
}

func TestUDPServerRoutesGenericRequests(t *testing.T) {
	runtime := NewRuntime(Config{
		UDPAddress:    ":0",
		ClientTimeout: time.Second,
	})
	err := runtime.RegisterFactory("thing", ObjectFactoryFunc(func(ctx RequestContext) (Object, error) {
		return &testObject{id: ctx.Request.ObjectID}, nil
	}))
	if err != nil {
		t.Fatalf("register factory: %v", err)
	}

	server := NewUDPServer(runtime)
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1234}
	message, err := json.Marshal(RequestMessage{
		Sequence:   1,
		ObjectType: "thing",
		ObjectID:   "udp-alpha",
		Data:       json.RawMessage(`{"value":7}`),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	if err := server.handleMessage(addr, message); err != nil {
		t.Fatalf("handle udp message: %v", err)
	}

	object, ok := runtime.Object("thing", "udp-alpha")
	if !ok {
		t.Fatalf("expected udp request to create object")
	}

	thing := object.(*testObject)
	if thing.value != 7 {
		t.Fatalf("expected udp request value 7, got %d", thing.value)
	}
	if thing.clientID != "udp-127.0.0.1:1234" {
		t.Fatalf("expected udp client id, got %q", thing.clientID)
	}
}
