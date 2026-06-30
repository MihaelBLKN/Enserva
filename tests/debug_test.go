package tests

import (
	netobjects "Enserva/netObjects"
	"Enserva/network"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDebugStateIncludesRuntimeInternals verifies that debug state includes runtime details.
func TestDebugStateIncludesRuntimeInternals(t *testing.T) {
	server := network.NewServer(network.Config{
		DebugEnabled: true,
		DebugAddress: ":9911",
	})
	if err := netobjects.Register(server); err != nil {
		t.Fatalf("register example objects: %v", err)
	}
	if err := server.RegisterObject(netobjects.NewBuilding("tower")); err != nil {
		t.Fatalf("register building: %v", err)
	}

	state := server.DebugState()
	if !state.Config.DebugEnabled {
		t.Fatalf("expected debug config to be enabled")
	}
	if state.Config.DebugAddress != ":9911" {
		t.Fatalf("expected debug address :9911, got %q", state.Config.DebugAddress)
	}
	if state.Summary.ObjectCount != 2 {
		t.Fatalf("expected auth and building objects, got %d", state.Summary.ObjectCount)
	}
	if state.Runtime.FactoryCount != 2 {
		t.Fatalf("expected player and building factories, got %d", state.Runtime.FactoryCount)
	}
	if !state.Runtime.Authentication.Required {
		t.Fatalf("expected authentication to be required")
	}

	authObject, ok := findDebugObject(state, "player-auth", "default")
	if !ok {
		t.Fatalf("expected hidden auth object in debug state: %#v", state.Runtime.Objects)
	}
	if authObject.Visible {
		t.Fatalf("expected auth object to be marked hidden")
	}
	if !hasCapability(authObject, "authentication") {
		t.Fatalf("expected auth object capabilities to include authentication: %#v", authObject.Capabilities)
	}

	buildingObject, ok := findDebugObject(state, "building", "tower")
	if !ok {
		t.Fatalf("expected building object in debug state")
	}
	snapshot, ok := buildingObject.Snapshot.(map[string]any)
	if !ok {
		t.Fatalf("expected building snapshot map, got %#v", buildingObject.Snapshot)
	}
	if snapshot["id"] != "tower" {
		t.Fatalf("expected building snapshot id tower, got %#v", snapshot["id"])
	}
	if state.Features.InterestManagement.ObjectCount != 1 {
		t.Fatalf("expected one interest-managed game object, got %d", state.Features.InterestManagement.ObjectCount)
	}
}

// TestDebugHandlerServesStateAndUI verifies that the debug HTTP handler serves API and asset routes.
func TestDebugHandlerServesStateAndUI(t *testing.T) {
	server := network.NewServer(network.Config{DebugEnabled: true})
	handler := server.DebugHandler()

	stateResponse := httptest.NewRecorder()
	handler.ServeHTTP(stateResponse, httptest.NewRequest(http.MethodGet, "/debug/state", nil))
	if stateResponse.Code != http.StatusOK {
		t.Fatalf("expected state status 200, got %d", stateResponse.Code)
	}

	var state network.DebugState
	if err := json.Unmarshal(stateResponse.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode debug state: %v", err)
	}
	if state.Config.DebugAddress == "" {
		t.Fatalf("expected normalized debug config in response")
	}

	uiResponse := httptest.NewRecorder()
	handler.ServeHTTP(uiResponse, httptest.NewRequest(http.MethodGet, "/debug", nil))
	if uiResponse.Code != http.StatusOK {
		t.Fatalf("expected UI status 200, got %d", uiResponse.Code)
	}
	if !strings.Contains(uiResponse.Body.String(), "Enserva Debug") {
		t.Fatalf("expected debug UI HTML")
	}
	if !strings.Contains(uiResponse.Body.String(), "index.css") || !strings.Contains(uiResponse.Body.String(), "index.js") {
		t.Fatalf("expected debug UI to reference external CSS and JS files")
	}

	cssResponse := httptest.NewRecorder()
	handler.ServeHTTP(cssResponse, httptest.NewRequest(http.MethodGet, "/debug/index.css", nil))
	if cssResponse.Code != http.StatusOK {
		t.Fatalf("expected CSS status 200, got %d", cssResponse.Code)
	}
	if !strings.Contains(cssResponse.Body.String(), ":root") {
		t.Fatalf("expected debug CSS content")
	}

	jsResponse := httptest.NewRecorder()
	handler.ServeHTTP(jsResponse, httptest.NewRequest(http.MethodGet, "/debug/index.js", nil))
	if jsResponse.Code != http.StatusOK {
		t.Fatalf("expected JS status 200, got %d", jsResponse.Code)
	}
	if !strings.Contains(jsResponse.Body.String(), "fetchState") {
		t.Fatalf("expected debug JS content")
	}
}

// findDebugObject locates one object in grouped debug state.
func findDebugObject(state network.DebugState, objectType, objectID string) (network.DebugObjectState, bool) {
	for _, group := range state.Runtime.Objects {
		if group.ObjectType != objectType {
			continue
		}
		for _, object := range group.Objects {
			if object.ObjectID == objectID {
				return object, true
			}
		}
	}

	return network.DebugObjectState{}, false
}

// hasCapability reports whether object lists capability.
func hasCapability(object network.DebugObjectState, capability string) bool {
	for _, candidate := range object.Capabilities {
		if candidate == capability {
			return true
		}
	}

	return false
}
