package network

import (
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// InterestSubject identifies how an object participates in interest management.
type InterestSubject string

// Features contains optional runtime systems shared with object handlers.
type Features struct {
	mu       sync.RWMutex
	interest *InterestManager
}

const (
	// InterestPlayer marks an object as a player whose position filters snapshots.
	InterestPlayer InterestSubject = "Player"
	// InterestGameObject marks an object as a position-aware object that may be filtered.
	InterestGameObject InterestSubject = "GameObject"
)

// InterestManagementConfig describes how an object's snapshot position is read.
type InterestManagementConfig struct {
	SubjectType InterestSubject
	ObjectType  string
	ObjectID    string
	XField      string
	YField      string
	ZField      string
	Radius      float64
	IncludeSelf bool
}

// PlayerInterest creates an interest configuration for a player object.
func PlayerInterest(object Object, xField, yField, zField string, radius float64) InterestManagementConfig {
	config := objectInterestConfig(InterestPlayer, object, xField, yField, zField)
	config.Radius = radius
	config.IncludeSelf = true
	return config
}

// PlayerInterest2D creates a two-dimensional interest configuration for a player object.
func PlayerInterest2D(object Object, xField, yField string, radius float64) InterestManagementConfig {
	return PlayerInterest(object, xField, yField, "", radius)
}

// GameObjectInterest creates an interest configuration for a non-player object.
func GameObjectInterest(object Object, xField, yField, zField string) InterestManagementConfig {
	return objectInterestConfig(InterestGameObject, object, xField, yField, zField)
}

// GameObjectInterest2D creates a two-dimensional interest configuration for a non-player object.
func GameObjectInterest2D(object Object, xField, yField string) InterestManagementConfig {
	return GameObjectInterest(object, xField, yField, "")
}

// InterestManager stores the active interest-management registrations.
type InterestManager struct {
	enabled bool
	players map[string]InterestManagementConfig
	objects map[string]InterestManagementConfig
}

type interestPosition struct {
	X    float64
	Y    float64
	Z    float64
	HasZ bool
}

type interestSnapshotObject struct {
	objectType  string
	objectID    string
	key         string
	snapshot    any
	visible     bool
	managed     bool
	config      InterestManagementConfig
	position    interestPosition
	hasPosition bool
}

type interestSpatialCell struct {
	x int
	y int
}

type interestSpatialHash struct {
	cellSize float64
	cells    map[interestSpatialCell][]int
}

// InterestManagement returns the manager, creating it when features is non-nil.
func (features *Features) InterestManagement() *InterestManager {
	if features == nil {
		return nil
	}

	features.mu.Lock()
	defer features.mu.Unlock()

	if features.interest == nil {
		features.interest = newInterestManager()
	}

	return features.interest
}

// EnableInterestManagement registers config and enables filtering once a player is configured.
func (features *Features) EnableInterestManagement(config InterestManagementConfig) {
	if features == nil {
		return
	}

	config, ok := normalizeInterestConfig(config)
	if !ok {
		return
	}

	features.mu.Lock()
	defer features.mu.Unlock()

	if features.interest == nil {
		features.interest = newInterestManager()
	}

	key := interestObjectKey(config.ObjectType, config.ObjectID)
	switch config.SubjectType {
	case InterestPlayer:
		features.interest.players[key] = config
	case InterestGameObject:
		features.interest.objects[key] = config
	}

	features.interest.enabled = len(features.interest.players) > 0
}

// DisableInterestManagement removes any interest registration for objectType and objectID.
func (features *Features) DisableInterestManagement(objectType, objectID string) {
	if features == nil {
		return
	}

	objectType = normalizeObjectKey(objectType)
	objectID = normalizeObjectKey(objectID)
	if objectType == "" || objectID == "" {
		return
	}

	features.mu.Lock()
	defer features.mu.Unlock()

	if features.interest == nil {
		return
	}

	key := interestObjectKey(objectType, objectID)
	delete(features.interest.players, key)
	delete(features.interest.objects, key)
	features.interest.enabled = len(features.interest.players) > 0
}

// extractPosition reads configured coordinate fields from a snapshot.
func extractPosition(snapshot any, config InterestManagementConfig) (interestPosition, bool) {
	x, ok := snapshotNumber(snapshot, config.XField)
	if !ok {
		return interestPosition{}, false
	}

	y, ok := snapshotNumber(snapshot, config.YField)
	if !ok {
		return interestPosition{}, false
	}

	position := interestPosition{
		X: x,
		Y: y,
	}

	if config.ZField == "" {
		return position, true
	}

	z, ok := snapshotNumber(snapshot, config.ZField)
	if !ok {
		return interestPosition{}, false
	}

	position.Z = z
	position.HasZ = true
	return position, true
}

// withinInterestRadius reports whether b is within radius of a.
func withinInterestRadius(a, b interestPosition, radius float64) bool {
	dx := a.X - b.X
	dy := a.Y - b.Y

	if a.HasZ && b.HasZ {
		dz := a.Z - b.Z
		return (dx*dx + dy*dy + dz*dz) <= (radius * radius)
	}

	return (dx*dx + dy*dy) <= (radius * radius)
}

// newInterestSpatialHash creates a grid index for interest broad-phase lookups.
func newInterestSpatialHash(cellSize float64) interestSpatialHash {
	if cellSize <= 0 || math.IsNaN(cellSize) || math.IsInf(cellSize, 0) {
		return interestSpatialHash{}
	}

	return interestSpatialHash{
		cellSize: cellSize,
		cells:    map[interestSpatialCell][]int{},
	}
}

// insert adds one indexed object position to the spatial hash.
func (hash interestSpatialHash) insert(position interestPosition, index int) {
	if hash.cellSize <= 0 || hash.cells == nil || !finiteInterestPosition(position) {
		return
	}

	cell := hash.cellFor(position.X, position.Y)
	hash.cells[cell] = append(hash.cells[cell], index)
}

// query returns object indexes from cells intersecting radius around position.
func (hash interestSpatialHash) query(position interestPosition, radius float64) []int {
	if hash.cellSize <= 0 || len(hash.cells) == 0 || radius <= 0 || math.IsNaN(radius) || math.IsInf(radius, 0) || !finiteInterestPosition(position) {
		return nil
	}

	minCell := hash.cellFor(position.X-radius, position.Y-radius)
	maxCell := hash.cellFor(position.X+radius, position.Y+radius)
	indexes := make([]int, 0)
	for y := minCell.y; y <= maxCell.y; y++ {
		for x := minCell.x; x <= maxCell.x; x++ {
			indexes = append(indexes, hash.cells[interestSpatialCell{x: x, y: y}]...)
		}
	}

	return indexes
}

// cellFor maps a world position to its spatial hash cell.
func (hash interestSpatialHash) cellFor(x, y float64) interestSpatialCell {
	return interestSpatialCell{
		x: int(math.Floor(x / hash.cellSize)),
		y: int(math.Floor(y / hash.cellSize)),
	}
}

// finiteInterestPosition reports whether position can be indexed in the spatial hash.
func finiteInterestPosition(position interestPosition) bool {
	if math.IsNaN(position.X) || math.IsNaN(position.Y) || math.IsInf(position.X, 0) || math.IsInf(position.Y, 0) {
		return false
	}
	if !position.HasZ {
		return true
	}

	return !math.IsNaN(position.Z) && !math.IsInf(position.Z, 0)
}

// effectiveInterestRadius returns the radius used to compare objectConfig to the player.
func effectiveInterestRadius(playerConfig, objectConfig InterestManagementConfig) float64 {
	radius := playerConfig.Radius
	if radius <= 0 {
		radius = objectConfig.Radius
	}

	return radius
}

// filtersByInterestRadius reports whether radius should spatially filter an object.
func filtersByInterestRadius(radius float64) bool {
	return radius > 0 && !math.IsNaN(radius) && !math.IsInf(radius, 0)
}

// interestSpatialQueryRadius returns the largest radius needed for a spatial lookup.
func interestSpatialQueryRadius(entries []interestSnapshotObject, playerConfig InterestManagementConfig) float64 {
	if filtersByInterestRadius(playerConfig.Radius) {
		return playerConfig.Radius
	}

	radius := 0.0
	for _, entry := range entries {
		if !entry.visible || !entry.managed {
			continue
		}
		entryRadius := effectiveInterestRadius(playerConfig, entry.config)
		if filtersByInterestRadius(entryRadius) && entryRadius > radius {
			radius = entryRadius
		}
	}

	return radius
}

type interestState struct {
	enabled bool
	players map[string]InterestManagementConfig
	objects map[string]InterestManagementConfig
}

// newInterestManager creates an empty manager.
func newInterestManager() *InterestManager {
	return &InterestManager{
		players: map[string]InterestManagementConfig{},
		objects: map[string]InterestManagementConfig{},
	}
}

// interestState returns a snapshot of active interest-management settings.
func (features *Features) interestState() interestState {
	if features == nil {
		return interestState{}
	}

	features.mu.RLock()
	defer features.mu.RUnlock()

	if features.interest == nil || !features.interest.enabled {
		return interestState{}
	}

	state := interestState{
		enabled: features.interest.enabled,
		players: make(map[string]InterestManagementConfig, len(features.interest.players)),
		objects: make(map[string]InterestManagementConfig, len(features.interest.objects)),
	}
	for key, config := range features.interest.players {
		state.players[key] = config
	}
	for key, config := range features.interest.objects {
		state.objects[key] = config
	}

	return state
}

// playerForClient finds the player interest config bound to clientID.
func (state interestState) playerForClient(clientID string) (InterestManagementConfig, bool) {
	clientID = normalizeObjectKey(clientID)
	if clientID == "" {
		return InterestManagementConfig{}, false
	}

	for _, config := range state.players {
		if config.ObjectID == clientID {
			return config, true
		}
	}

	return InterestManagementConfig{}, false
}

// objectConfig returns the interest config for an object when one is registered.
func (state interestState) objectConfig(objectType, objectID string) (InterestManagementConfig, bool) {
	key := interestObjectKey(objectType, objectID)
	if config, ok := state.players[key]; ok {
		return config, true
	}
	if config, ok := state.objects[key]; ok {
		return config, true
	}

	return InterestManagementConfig{}, false
}

// normalizeInterestConfig validates and canonicalizes an interest config.
func normalizeInterestConfig(config InterestManagementConfig) (InterestManagementConfig, bool) {
	config.ObjectType = normalizeObjectKey(config.ObjectType)
	config.ObjectID = normalizeObjectKey(config.ObjectID)
	config.XField = strings.TrimSpace(config.XField)
	config.YField = strings.TrimSpace(config.YField)
	config.ZField = strings.TrimSpace(config.ZField)
	if config.ObjectType == "" || config.ObjectID == "" || config.XField == "" || config.YField == "" {
		return InterestManagementConfig{}, false
	}
	if config.Radius < 0 {
		config.Radius = 0
	}

	switch strings.ToLower(strings.TrimSpace(string(config.SubjectType))) {
	case "player":
		config.SubjectType = InterestPlayer
	case "gameobject", "game object", "object":
		config.SubjectType = InterestGameObject
	default:
		return InterestManagementConfig{}, false
	}

	return config, true
}

// objectInterestConfig builds a config from an object's identity.
func objectInterestConfig(subject InterestSubject, object Object, xField, yField, zField string) InterestManagementConfig {
	config := InterestManagementConfig{
		SubjectType: subject,
		XField:      xField,
		YField:      yField,
		ZField:      zField,
	}

	objectType, objectID, err := objectIdentity(object)
	if err == nil {
		config.ObjectType = objectType
		config.ObjectID = objectID
	}

	return config
}

// interestObjectKey returns the map key for an object identity.
func interestObjectKey(objectType, objectID string) string {
	return normalizeObjectKey(objectType) + "/" + normalizeObjectKey(objectID)
}

// snapshotNumber extracts a numeric field from map or struct snapshots.
func snapshotNumber(snapshot any, fieldName string) (float64, bool) {
	fieldName = strings.TrimSpace(fieldName)
	if fieldName == "" {
		return 0, false
	}

	value := indirectValue(reflect.ValueOf(snapshot))
	if !value.IsValid() {
		return 0, false
	}

	switch value.Kind() {
	case reflect.Map:
		return mapNumber(value, fieldName)
	case reflect.Struct:
		return structNumber(value, fieldName)
	default:
		return 0, false
	}
}

// mapNumber finds a numeric value in a string-keyed map.
func mapNumber(value reflect.Value, fieldName string) (float64, bool) {
	if value.Type().Key().Kind() != reflect.String {
		return 0, false
	}

	for _, key := range value.MapKeys() {
		keyName := key.String()
		if keyName != fieldName && !strings.EqualFold(keyName, fieldName) {
			continue
		}

		return numberValue(value.MapIndex(key))
	}

	return 0, false
}

// structNumber finds a numeric exported field by Go name or JSON tag.
func structNumber(value reflect.Value, fieldName string) (float64, bool) {
	valueType := value.Type()
	for i := 0; i < value.NumField(); i++ {
		field := valueType.Field(i)
		if field.PkgPath != "" {
			continue
		}

		if matchesSnapshotField(field, fieldName) {
			return numberValue(value.Field(i))
		}
	}

	return 0, false
}

// matchesSnapshotField reports whether field matches a configured snapshot field name.
func matchesSnapshotField(field reflect.StructField, fieldName string) bool {
	if field.Name == fieldName || strings.EqualFold(field.Name, fieldName) {
		return true
	}

	jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
	if jsonName == "" || jsonName == "-" {
		return false
	}

	return jsonName == fieldName || strings.EqualFold(jsonName, fieldName)
}

// numberValue converts common numeric snapshot values to float64.
func numberValue(value reflect.Value) (float64, bool) {
	value = indirectValue(value)
	if !value.IsValid() {
		return 0, false
	}

	if value.Type() == reflect.TypeOf(json.Number("")) {
		number := value.Interface().(json.Number)
		parsed, err := number.Float64()
		return parsed, err == nil
	}

	switch value.Kind() {
	case reflect.Float32, reflect.Float64:
		return value.Convert(reflect.TypeOf(float64(0))).Float(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(value.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return float64(value.Uint()), true
	case reflect.String:
		parsed, err := strconv.ParseFloat(value.String(), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

// indirectValue unwraps interfaces and pointers until it reaches a concrete value.
func indirectValue(value reflect.Value) reflect.Value {
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer) {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}

	return value
}
