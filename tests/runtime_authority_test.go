package tests

import (
	"Enserva/network"
	"encoding/json"
	"errors"
	"testing"
)

type authorityTestObject struct {
	id       string
	value    int
	requests int
}

func (object *authorityTestObject) ObjectType() string {
	return "thing"
}

func (object *authorityTestObject) ObjectID() string {
	return object.id
}

func (object *authorityTestObject) Snapshot() any {
	return map[string]any{
		"id":       object.id,
		"value":    object.value,
		"requests": object.requests,
	}
}

func (object *authorityTestObject) OnRequest(ctx network.RequestContext) error {
	var payload struct {
		Value int `json:"value"`
	}
	if err := ctx.Decode(&payload); err != nil {
		return err
	}

	object.value = payload.Value
	object.requests++
	return nil
}

func TestRuntimeDoesNotCreateObjectsFromClientRequests(t *testing.T) {
	runtime := network.NewRuntime(network.Config{})
	factoryCalls := 0

	if err := runtime.RegisterFactory("thing", network.ObjectFactoryFunc(func(ctx network.RequestContext) (network.Object, error) {
		factoryCalls++
		return &authorityTestObject{id: ctx.Request.ObjectID}, nil
	})); err != nil {
		t.Fatalf("register factory: %v", err)
	}

	err := runtime.HandleRequest(network.RequestContext{
		ClientID: "client-a",
		Request: network.RequestMessage{
			ObjectType: "thing",
			ObjectID:   "alpha",
			Data:       json.RawMessage(`{"value":42}`),
		},
	})
	if !errors.Is(err, network.ErrObjectNotFound) {
		t.Fatalf("expected missing object error, got %v", err)
	}
	if factoryCalls != 0 {
		t.Fatalf("expected client request not to call factory, got %d calls", factoryCalls)
	}
	if _, ok := runtime.Object("thing", "alpha"); ok {
		t.Fatalf("client request created object alpha")
	}

	object, err := runtime.CreateObject("thing", "alpha")
	if err != nil {
		t.Fatalf("server create object: %v", err)
	}

	err = runtime.HandleRequest(network.RequestContext{
		ClientID: "client-a",
		Request: network.RequestMessage{
			ObjectType: "thing",
			ObjectID:   "alpha",
			Data:       json.RawMessage(`{"value":7}`),
		},
	})
	if err != nil {
		t.Fatalf("handle request for server-created object: %v", err)
	}

	thing := object.(*authorityTestObject)
	if thing.value != 7 {
		t.Fatalf("expected request to update existing object, got %d", thing.value)
	}
	if thing.requests != 1 {
		t.Fatalf("expected one routed request, got %d", thing.requests)
	}
}
