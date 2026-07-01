package network

import (
	debugfrontend "Enserva/debugFrontend"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"time"
)

// DebugState is the complete JSON payload served by the debug endpoint.
type DebugState struct {
	GeneratedAt time.Time         `json:"generatedAt"`
	URL         string            `json:"url"`
	Summary     DebugSummary      `json:"summary"`
	Config      DebugConfigState  `json:"config"`
	Runtime     DebugRuntimeState `json:"runtime"`
	Features    DebugFeatureState `json:"features"`
	UDP         DebugUDPState     `json:"udp"`
}

// DebugSummary contains high-level counts and feature flags for the runtime.
type DebugSummary struct {
	Tick                   uint64 `json:"tick"`
	ObjectCount            int    `json:"objectCount"`
	ObjectTypeCount        int    `json:"objectTypeCount"`
	FactoryCount           int    `json:"factoryCount"`
	UDPClientCount         int    `json:"udpClientCount"`
	AuthenticatedUDPClient int    `json:"authenticatedUdpClientCount"`
	AuthenticationRequired bool   `json:"authenticationRequired"`
	InterestEnabled        bool   `json:"interestEnabled"`
	ScenesEnabled          bool   `json:"scenesEnabled"`
}

// DebugConfigState contains normalized configuration values for display.
type DebugConfigState struct {
	UDPAddress                string  `json:"udpAddress"`
	TickRate                  int     `json:"tickRate"`
	TickInterval              string  `json:"tickInterval"`
	TickIntervalMs            float64 `json:"tickIntervalMs"`
	SnapshotRate              int     `json:"snapshotRate"`
	SnapshotEvery             uint64  `json:"snapshotEvery"`
	EnableDeltaSnapshots      bool    `json:"enableDeltaSnapshots"`
	SupportedWireCapabilities uint64  `json:"supportedWireCapabilities"`
	FullSnapshotInterval      int     `json:"fullSnapshotInterval"`
	ClientTimeout             string  `json:"clientTimeout"`
	ClientTimeoutMs           int64   `json:"clientTimeoutMs"`
	MaxClients                int     `json:"maxClients"`
	MaxUDPPacketSize          int     `json:"maxUdpPacketSize"`
	EnableBandwidthBudget     bool    `json:"enableBandwidthBudget"`
	ClientBytesPerSecond      int     `json:"clientBytesPerSecond"`
	DefaultSnapshotPriority   int     `json:"defaultSnapshotPriority"`
	ReliableRetryInterval     string  `json:"reliableRetryInterval"`
	ReliableRetryIntervalMs   float64 `json:"reliableRetryIntervalMs"`
	ReliableMaxAttempts       int     `json:"reliableMaxAttempts"`
	ReliableQueueLimit        int     `json:"reliableQueueLimit"`
	MaxInputFutureTicks       uint64  `json:"maxInputFutureTicks"`
	MaxInputPastTicks         uint64  `json:"maxInputPastTicks"`
	InputBufferLimit          int     `json:"inputBufferLimit"`
	DebugEnabled              bool    `json:"debugEnabled"`
	DebugAddress              string  `json:"debugAddress"`
	DebugURL                  string  `json:"debugUrl"`
}

// DebugRuntimeState contains object, factory, and authentication state for debugging.
type DebugRuntimeState struct {
	Tick           uint64                    `json:"tick"`
	ObjectCount    int                       `json:"objectCount"`
	ObjectTypes    int                       `json:"objectTypes"`
	FactoryCount   int                       `json:"factoryCount"`
	Factories      []DebugFactoryState       `json:"factories"`
	Authentication DebugAuthenticationState  `json:"authentication"`
	Metrics        RuntimeMetrics            `json:"metrics"`
	InputBuffer    InputBufferMetrics        `json:"inputBuffer"`
	Objects        []DebugObjectTypeState    `json:"objects"`
	ObjectsByType  map[string]map[string]any `json:"objectsByType"`
}

// DebugFactoryState describes a registered object factory.
type DebugFactoryState struct {
	ObjectType string `json:"objectType"`
	GoType     string `json:"goType"`
}

// DebugAuthenticationState describes the registered authentication handler.
type DebugAuthenticationState struct {
	Required   bool   `json:"required"`
	ObjectType string `json:"objectType,omitempty"`
	ObjectID   string `json:"objectId,omitempty"`
	GoType     string `json:"goType,omitempty"`
}

// DebugObjectTypeState groups debug object state by object type.
type DebugObjectTypeState struct {
	ObjectType string             `json:"objectType"`
	Count      int                `json:"count"`
	Objects    []DebugObjectState `json:"objects"`
}

// DebugObjectState describes one runtime object for the debug interface.
type DebugObjectState struct {
	ObjectType    string   `json:"objectType"`
	ObjectID      string   `json:"objectId"`
	GoType        string   `json:"goType"`
	Visible       bool     `json:"visible"`
	Capabilities  []string `json:"capabilities"`
	Snapshot      any      `json:"snapshot"`
	SnapshotError string   `json:"snapshotError,omitempty"`
}

// DebugFeatureState contains optional feature state.
type DebugFeatureState struct {
	InterestManagement DebugInterestState `json:"interestManagement"`
	SceneManagement    DebugSceneState    `json:"sceneManagement"`
}

// DebugInterestState describes active interest-management registrations.
type DebugInterestState struct {
	Configured  bool                  `json:"configured"`
	Enabled     bool                  `json:"enabled"`
	PlayerCount int                   `json:"playerCount"`
	ObjectCount int                   `json:"objectCount"`
	Players     []DebugInterestConfig `json:"players"`
	Objects     []DebugInterestConfig `json:"objects"`
}

// DebugInterestConfig describes one interest-management registration.
type DebugInterestConfig struct {
	Key         string  `json:"key"`
	SubjectType string  `json:"subjectType"`
	ObjectType  string  `json:"objectType"`
	ObjectID    string  `json:"objectId"`
	XField      string  `json:"xField"`
	YField      string  `json:"yField"`
	ZField      string  `json:"zField,omitempty"`
	Radius      float64 `json:"radius"`
	IncludeSelf bool    `json:"includeSelf"`
}

// DebugSceneState describes active scene-management registrations.
type DebugSceneState struct {
	Configured   bool              `json:"configured"`
	Enabled      bool              `json:"enabled"`
	ClientCount  int               `json:"clientCount"`
	ObjectCount  int               `json:"objectCount"`
	ClientScenes []DebugSceneEntry `json:"clientScenes"`
	ObjectScenes []DebugSceneEntry `json:"objectScenes"`
}

// DebugSceneEntry describes one scene assignment.
type DebugSceneEntry struct {
	Key   string  `json:"key"`
	Scene SceneID `json:"scene"`
}

// DebugUDPState contains UDP transport state and counters.
type DebugUDPState struct {
	Address                  string           `json:"address"`
	Started                  bool             `json:"started"`
	StartedAt                time.Time        `json:"startedAt,omitempty"`
	Uptime                   string           `json:"uptime,omitempty"`
	UptimeSeconds            float64          `json:"uptimeSeconds,omitempty"`
	ClientCount              int              `json:"clientCount"`
	AuthenticatedClientCount int              `json:"authenticatedClientCount"`
	ClientTimeout            string           `json:"clientTimeout"`
	Counters                 DebugUDPCounters `json:"counters"`
	Clients                  []DebugUDPClient `json:"clients"`
}

// DebugUDPCounters contains cumulative UDP transport counters.
type DebugUDPCounters struct {
	DatagramsReceived               uint64  `json:"datagramsReceived"`
	RequestsAccepted                uint64  `json:"requestsAccepted"`
	RequestsDropped                 uint64  `json:"requestsDropped"`
	RequestErrors                   uint64  `json:"requestErrors"`
	AuthAttempts                    uint64  `json:"authAttempts"`
	AuthSuccesses                   uint64  `json:"authSuccesses"`
	AuthFailures                    uint64  `json:"authFailures"`
	SnapshotsSent                   uint64  `json:"snapshotsSent"`
	FullSnapshotsSent               uint64  `json:"fullSnapshotsSent"`
	DeltaSnapshotsSent              uint64  `json:"deltaSnapshotsSent"`
	SnapshotErrors                  uint64  `json:"snapshotErrors"`
	OversizedOutbound               uint64  `json:"oversizedOutboundPacketsDropped"`
	ReliableQueued                  uint64  `json:"reliableMessagesQueued"`
	ReliableRetransmits             uint64  `json:"reliableRetransmits"`
	ReliableDrops                   uint64  `json:"reliableDrops"`
	ReliableAckRemovals             uint64  `json:"reliableAckRemovals"`
	BudgetDrops                     uint64  `json:"bandwidthBudgetDrops"`
	BudgetDeferrals                 uint64  `json:"bandwidthBudgetDeferrals"`
	OutboundBytesSent               uint64  `json:"outboundBytesSent"`
	SnapshotEncodeCount             uint64  `json:"snapshotEncodeCount"`
	LastSnapshotEncodeDurationNs    int64   `json:"lastSnapshotEncodeDurationNs"`
	LastSnapshotEncodeDurationMs    float64 `json:"lastSnapshotEncodeDurationMs"`
	MaxSnapshotEncodeDurationNs     int64   `json:"maxSnapshotEncodeDurationNs"`
	MaxSnapshotEncodeDurationMs     float64 `json:"maxSnapshotEncodeDurationMs"`
	TotalSnapshotEncodeDurationNs   int64   `json:"totalSnapshotEncodeDurationNs"`
	TotalSnapshotEncodeDurationMs   float64 `json:"totalSnapshotEncodeDurationMs"`
	AverageSnapshotEncodeDurationMs float64 `json:"averageSnapshotEncodeDurationMs"`
	ClientsCreated                  uint64  `json:"clientsCreated"`
	ClientsRemoved                  uint64  `json:"clientsRemoved"`
}

// DebugUDPClient describes a known UDP client.
type DebugUDPClient struct {
	Address         string    `json:"address"`
	ConnectionID    string    `json:"connectionId"`
	ID              string    `json:"id"`
	Authenticated   bool      `json:"authenticated"`
	LastSequence    uint64    `json:"lastSeq"`
	ReliableQueued  int       `json:"reliableQueued"`
	BudgetDrops     uint64    `json:"bandwidthBudgetDrops"`
	BudgetDeferrals uint64    `json:"bandwidthBudgetDeferrals"`
	BytesSent       uint64    `json:"bytesSent"`
	LastHeardAt     time.Time `json:"lastHeardAt"`
	Idle            string    `json:"idle"`
	IdleSeconds     float64   `json:"idleSeconds"`
}

// ListenAndServeDebug starts the browser debug interface.
func (server *Server) ListenAndServeDebug() error {
	return http.ListenAndServe(server.config.DebugAddress, server.DebugHandler())
}

// DebugHandler returns the HTTP handler for debug state and static frontend assets.
func (server *Server) DebugHandler() http.Handler {
	frontend := debugNoStore(http.FileServer(http.FS(debugfrontend.FS())))

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/state", server.serveDebugState)
	mux.HandleFunc("/healthz", serveDebugHealth)
	mux.Handle("/debug/", http.StripPrefix("/debug", frontend))
	mux.HandleFunc("/debug", func(writer http.ResponseWriter, request *http.Request) {
		serveDebugFrontendPath(frontend, writer, request, "/")
	})
	mux.Handle("/", frontend)
	return mux
}

// DebugState returns a point-in-time view of server state for diagnostics.
func (server *Server) DebugState() DebugState {
	runtimeState := server.runtime.DebugState()
	featureState := server.runtime.Features().DebugState()
	udpState := DebugUDPState{
		Address:       server.config.UDPAddress,
		ClientTimeout: server.config.ClientTimeout.String(),
	}
	if udpServer := server.UDPServer(); udpServer != nil {
		udpState = udpServer.DebugState()
	}

	state := DebugState{
		GeneratedAt: time.Now(),
		URL:         debugHTTPURL(server.config.DebugAddress),
		Config:      debugConfigState(server.config),
		Runtime:     runtimeState,
		Features:    featureState,
		UDP:         udpState,
	}
	state.Summary = DebugSummary{
		Tick:                   runtimeState.Tick,
		ObjectCount:            runtimeState.ObjectCount,
		ObjectTypeCount:        runtimeState.ObjectTypes,
		FactoryCount:           runtimeState.FactoryCount,
		UDPClientCount:         udpState.ClientCount,
		AuthenticatedUDPClient: udpState.AuthenticatedClientCount,
		AuthenticationRequired: runtimeState.Authentication.Required,
		InterestEnabled:        featureState.InterestManagement.Enabled,
		ScenesEnabled:          featureState.SceneManagement.Enabled,
	}

	return state
}

// DebugState returns a point-in-time view of runtime internals for diagnostics.
func (runtime *Runtime) DebugState() DebugRuntimeState {
	runtime.hooksMu.Lock()
	defer runtime.hooksMu.Unlock()

	runtime.mu.RLock()
	tick := runtime.tick
	factories := debugFactoryStates(runtime.factories)
	authentication := DebugAuthenticationState{
		Required:   runtime.authenticationHandler != nil,
		ObjectType: runtime.authenticationObjectType,
		ObjectID:   runtime.authenticationObjectID,
	}
	if runtime.authenticationHandler != nil {
		authentication.GoType = debugGoType(runtime.authenticationHandler)
	}

	objectsByType := make(map[string][]Object, len(runtime.objects))
	for objectType, objectsByID := range runtime.objects {
		objectsByType[objectType] = make([]Object, 0, len(objectsByID))
		for _, object := range objectsByID {
			objectsByType[objectType] = append(objectsByType[objectType], object)
		}
	}
	runtime.mu.RUnlock()

	objectGroups, snapshotMap, objectCount := debugObjectGroups(objectsByType)
	return DebugRuntimeState{
		Tick:           tick,
		ObjectCount:    objectCount,
		ObjectTypes:    len(objectGroups),
		FactoryCount:   len(factories),
		Factories:      factories,
		Authentication: authentication,
		Metrics:        runtime.Metrics(),
		InputBuffer:    runtime.InputBufferMetrics(),
		Objects:        objectGroups,
		ObjectsByType:  snapshotMap,
	}
}

// DebugState returns a point-in-time view of optional feature state.
func (features *Features) DebugState() DebugFeatureState {
	return DebugFeatureState{
		InterestManagement: features.debugInterestState(),
		SceneManagement:    features.debugSceneState(),
	}
}

// debugInterestState returns a serializable snapshot of interest-management state.
func (features *Features) debugInterestState() DebugInterestState {
	if features == nil {
		return DebugInterestState{}
	}

	features.mu.RLock()
	defer features.mu.RUnlock()

	if features.interest == nil {
		return DebugInterestState{}
	}

	state := DebugInterestState{
		Configured:  true,
		Enabled:     features.interest.enabled,
		PlayerCount: len(features.interest.players),
		ObjectCount: len(features.interest.objects),
		Players:     debugInterestConfigs(features.interest.players),
		Objects:     debugInterestConfigs(features.interest.objects),
	}
	return state
}

// DebugState returns a point-in-time view of UDP transport state.
func (server *UDPServer) DebugState() DebugUDPState {
	if server == nil {
		return DebugUDPState{}
	}

	now := time.Now()
	server.mu.Lock()
	defer server.mu.Unlock()

	clients := make([]DebugUDPClient, 0, len(server.clients))
	authenticatedClients := 0
	for _, client := range server.clients {
		idle := now.Sub(client.lastHeardAt)
		if client.authenticated {
			authenticatedClients++
		}
		clients = append(clients, DebugUDPClient{
			Address:         client.addr.String(),
			ConnectionID:    client.connectionID,
			ID:              client.id,
			Authenticated:   client.authenticated,
			LastSequence:    client.lastSeq,
			ReliableQueued:  len(client.reliableOut),
			BudgetDrops:     client.budgetDrops,
			BudgetDeferrals: client.budgetDeferrals,
			BytesSent:       client.bytesSent,
			LastHeardAt:     client.lastHeardAt,
			Idle:            idle.Truncate(time.Millisecond).String(),
			IdleSeconds:     idle.Seconds(),
		})
	}
	sort.Slice(clients, func(i, j int) bool {
		return clients[i].Address < clients[j].Address
	})

	uptime := now.Sub(server.startedAt)
	return DebugUDPState{
		Address:                  server.address,
		Started:                  !server.startedAt.IsZero(),
		StartedAt:                server.startedAt,
		Uptime:                   uptime.Truncate(time.Millisecond).String(),
		UptimeSeconds:            uptime.Seconds(),
		ClientCount:              len(clients),
		AuthenticatedClientCount: authenticatedClients,
		ClientTimeout:            server.runtime.Config().ClientTimeout.String(),
		Counters: DebugUDPCounters{
			DatagramsReceived:               server.datagramsReceived,
			RequestsAccepted:                server.requestsAccepted,
			RequestsDropped:                 server.requestsDropped,
			RequestErrors:                   server.requestErrors,
			AuthAttempts:                    server.authAttempts,
			AuthSuccesses:                   server.authSuccesses,
			AuthFailures:                    server.authFailures,
			SnapshotsSent:                   server.snapshotsSent,
			FullSnapshotsSent:               server.fullSnapshotsSent,
			DeltaSnapshotsSent:              server.deltaSnapshotsSent,
			SnapshotErrors:                  server.snapshotErrors,
			OversizedOutbound:               server.oversizedOutbound,
			ReliableQueued:                  server.reliableQueued,
			ReliableRetransmits:             server.reliableRetransmits,
			ReliableDrops:                   server.reliableDrops,
			ReliableAckRemovals:             server.reliableAckRemovals,
			BudgetDrops:                     server.budgetDrops,
			BudgetDeferrals:                 server.budgetDeferrals,
			OutboundBytesSent:               server.outboundBytesSent,
			SnapshotEncodeCount:             server.snapshotEncodeCount,
			LastSnapshotEncodeDurationNs:    server.lastSnapshotEncodeDurationNs,
			LastSnapshotEncodeDurationMs:    durationMillis(server.lastSnapshotEncodeDurationNs),
			MaxSnapshotEncodeDurationNs:     server.maxSnapshotEncodeDurationNs,
			MaxSnapshotEncodeDurationMs:     durationMillis(server.maxSnapshotEncodeDurationNs),
			TotalSnapshotEncodeDurationNs:   server.totalSnapshotEncodeDurationNs,
			TotalSnapshotEncodeDurationMs:   durationMillis(server.totalSnapshotEncodeDurationNs),
			AverageSnapshotEncodeDurationMs: snapshotAverageDurationMillis(server.totalSnapshotEncodeDurationNs, server.snapshotEncodeCount),
			ClientsCreated:                  server.clientsCreated,
			ClientsRemoved:                  server.clientsRemoved,
		},
		Clients: clients,
	}
}

// debugConfigState returns normalized configuration values for the debug endpoint.
func debugConfigState(config Config) DebugConfigState {
	config = config.Normalized()
	tickInterval := config.TickInterval()
	return DebugConfigState{
		UDPAddress:                config.UDPAddress,
		TickRate:                  config.TickRate,
		TickInterval:              tickInterval.String(),
		TickIntervalMs:            float64(tickInterval) / float64(time.Millisecond),
		SnapshotRate:              config.SnapshotRate,
		SnapshotEvery:             config.SnapshotEvery(),
		EnableDeltaSnapshots:      config.EnableDeltaSnapshots,
		SupportedWireCapabilities: config.SupportedWireCapabilities,
		FullSnapshotInterval:      config.FullSnapshotInterval,
		ClientTimeout:             config.ClientTimeout.String(),
		ClientTimeoutMs:           int64(config.ClientTimeout / time.Millisecond),
		MaxClients:                config.MaxClients,
		MaxUDPPacketSize:          config.MaxUDPPacketSize,
		EnableBandwidthBudget:     config.EnableBandwidthBudget,
		ClientBytesPerSecond:      config.ClientBytesPerSecond,
		DefaultSnapshotPriority:   int(config.DefaultSnapshotPriority),
		ReliableRetryInterval:     config.ReliableRetryInterval.String(),
		ReliableRetryIntervalMs:   float64(config.ReliableRetryInterval) / float64(time.Millisecond),
		ReliableMaxAttempts:       config.ReliableMaxAttempts,
		ReliableQueueLimit:        config.ReliableQueueLimit,
		MaxInputFutureTicks:       config.MaxInputFutureTicks,
		MaxInputPastTicks:         config.MaxInputPastTicks,
		InputBufferLimit:          config.InputBufferLimit,
		DebugEnabled:              config.DebugEnabled,
		DebugAddress:              config.DebugAddress,
		DebugURL:                  debugHTTPURL(config.DebugAddress),
	}
}

// debugFactoryStates returns sorted debug state for registered factories.
func debugFactoryStates(factories map[string]ObjectFactory) []DebugFactoryState {
	states := make([]DebugFactoryState, 0, len(factories))
	for objectType, factory := range factories {
		states = append(states, DebugFactoryState{
			ObjectType: objectType,
			GoType:     debugGoType(factory),
		})
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].ObjectType < states[j].ObjectType
	})

	return states
}

// debugObjectGroups groups objects and snapshot values by object type.
func debugObjectGroups(objectsByType map[string][]Object) ([]DebugObjectTypeState, map[string]map[string]any, int) {
	objectTypes := make([]string, 0, len(objectsByType))
	for objectType := range objectsByType {
		objectTypes = append(objectTypes, objectType)
	}
	sort.Strings(objectTypes)

	groups := make([]DebugObjectTypeState, 0, len(objectTypes))
	snapshots := map[string]map[string]any{}
	objectCount := 0
	for _, objectType := range objectTypes {
		objects := debugObjectStates(objectsByType[objectType])
		objectCount += len(objects)
		groups = append(groups, DebugObjectTypeState{
			ObjectType: objectType,
			Count:      len(objects),
			Objects:    objects,
		})
		snapshots[objectType] = map[string]any{}
		for _, object := range objects {
			snapshots[objectType][object.ObjectID] = object.Snapshot
		}
	}

	return groups, snapshots, objectCount
}

// debugObjectStates returns sorted debug state for objects.
func debugObjectStates(objects []Object) []DebugObjectState {
	states := make([]DebugObjectState, 0, len(objects))
	for _, object := range objects {
		objectType, objectID, err := objectIdentity(object)
		if err != nil {
			states = append(states, DebugObjectState{
				ObjectType:    "",
				ObjectID:      "",
				GoType:        debugGoType(object),
				Visible:       false,
				Capabilities:  debugCapabilities(object),
				SnapshotError: err.Error(),
			})
			continue
		}

		snapshot, snapshotError := debugObjectSnapshot(object)
		visible, visibleError := debugSnapshotVisible(object)
		if visibleError != "" {
			if snapshotError != "" {
				snapshotError += ", "
			}
			snapshotError += visibleError
		}

		states = append(states, DebugObjectState{
			ObjectType:    objectType,
			ObjectID:      objectID,
			GoType:        debugGoType(object),
			Visible:       visible,
			Capabilities:  debugCapabilities(object),
			Snapshot:      snapshot,
			SnapshotError: snapshotError,
		})
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].ObjectID < states[j].ObjectID
	})

	return states
}

// debugInterestConfigs returns sorted debug state for interest configs.
func debugInterestConfigs(configs map[string]InterestManagementConfig) []DebugInterestConfig {
	keys := make([]string, 0, len(configs))
	for key := range configs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	states := make([]DebugInterestConfig, 0, len(keys))
	for _, key := range keys {
		config := configs[key]
		states = append(states, DebugInterestConfig{
			Key:         key,
			SubjectType: string(config.SubjectType),
			ObjectType:  config.ObjectType,
			ObjectID:    config.ObjectID,
			XField:      config.XField,
			YField:      config.YField,
			ZField:      config.ZField,
			Radius:      config.Radius,
			IncludeSelf: config.IncludeSelf,
		})
	}

	return states
}

// debugSceneState returns a serializable snapshot of scene-management state.
func (features *Features) debugSceneState() DebugSceneState {
	if features == nil {
		return DebugSceneState{}
	}

	features.mu.RLock()
	defer features.mu.RUnlock()

	if features.scenes == nil {
		return DebugSceneState{}
	}

	return DebugSceneState{
		Configured:   true,
		Enabled:      features.scenes.enabled,
		ClientCount:  len(features.scenes.clients),
		ObjectCount:  len(features.scenes.objects),
		ClientScenes: debugSceneEntries(features.scenes.clients),
		ObjectScenes: debugSceneEntries(features.scenes.objects),
	}
}

// debugSceneEntries returns sorted debug state for scene assignments.
func debugSceneEntries(entries map[string]SceneID) []DebugSceneEntry {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	states := make([]DebugSceneEntry, 0, len(keys))
	for _, key := range keys {
		states = append(states, DebugSceneEntry{
			Key:   key,
			Scene: entries[key],
		})
	}

	return states
}

// debugObjectSnapshot converts an object's snapshot to JSON-compatible data.
func debugObjectSnapshot(object Object) (snapshot any, snapshotError string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			snapshot = nil
			snapshotError = fmt.Sprintf("snapshot panic: %v", recovered)
		}
	}()

	return debugJSONValue(object.Snapshot())
}

// debugJSONValue round-trips value through JSON to match API output shapes.
func debugJSONValue(value any) (any, string) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err.Error()
	}

	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err.Error()
	}

	return decoded, ""
}

// debugSnapshotVisible reports snapshot visibility while recovering panics.
func debugSnapshotVisible(object Object) (visible bool, visibleError string) {
	visibility, ok := object.(SnapshotVisibility)
	if !ok {
		return true, ""
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			visible = false
			visibleError = fmt.Sprintf("visibility panic: %v", recovered)
		}
	}()

	return visibility.SnapshotVisible(), ""
}

// debugCapabilities returns the runtime hooks implemented by object.
func debugCapabilities(object Object) []string {
	capabilities := []string{"snapshot"}
	if _, ok := object.(InitHandler); ok {
		capabilities = append(capabilities, "init")
	}
	if _, ok := object.(TickHandler); ok {
		capabilities = append(capabilities, "tick")
	}
	if _, ok := object.(FullTickHandler); ok {
		capabilities = append(capabilities, "fullTick")
	}
	if _, ok := object.(RequestHandler); ok {
		capabilities = append(capabilities, "request")
	}
	if _, ok := object.(AuthenticationHandler); ok {
		capabilities = append(capabilities, "authentication")
	}
	if _, ok := object.(SnapshotVisibility); ok {
		capabilities = append(capabilities, "visibility")
	}
	if _, ok := object.(SceneSwitchHandler); ok {
		capabilities = append(capabilities, "sceneSwitch")
	}

	return capabilities
}

// debugGoType returns the concrete Go type name for value.
func debugGoType(value any) string {
	if value == nil {
		return "<nil>"
	}

	valueType := reflect.TypeOf(value)
	if valueType == nil {
		return "<nil>"
	}

	return valueType.String()
}

// serveDebugState writes the current debug state as JSON.
func (server *Server) serveDebugState(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(server.DebugState()); err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

// serveDebugHealth writes the debug health-check response.
func serveDebugHealth(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	_, _ = writer.Write([]byte("ok\n"))
}

// debugHTTPURL converts a configured address into a browser-friendly URL.
func debugHTTPURL(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		address = defaultDebugAddress
	}
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return address
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		if strings.HasPrefix(address, ":") {
			return "http://localhost" + address
		}
		return "http://" + address
	}

	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "localhost"
	}

	return "http://" + net.JoinHostPort(host, port)
}

// debugNoStore adds no-store caching headers before delegating to handler.
func debugNoStore(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		handler.ServeHTTP(writer, request)
	})
}

// serveDebugFrontendPath serves a static frontend path for a debug route.
func serveDebugFrontendPath(handler http.Handler, writer http.ResponseWriter, request *http.Request, path string) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	staticRequest := request.Clone(request.Context())
	urlCopy := *request.URL
	urlCopy.Path = path
	urlCopy.RawPath = ""
	staticRequest.URL = &urlCopy
	handler.ServeHTTP(writer, staticRequest)
}
