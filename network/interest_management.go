package network

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

type InterestSubject string

type Features struct {
	mu       sync.RWMutex
	interest *InterestManager
}

const (
	InterestPlayer     InterestSubject = "Player"
	InterestGameObject InterestSubject = "GameObject"
)

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

func PlayerInterest(object Object, xField, yField, zField string, radius float64) InterestManagementConfig {
	config := objectInterestConfig(InterestPlayer, object, xField, yField, zField)
	config.Radius = radius
	config.IncludeSelf = true
	return config
}

func PlayerInterest2D(object Object, xField, yField string, radius float64) InterestManagementConfig {
	return PlayerInterest(object, xField, yField, "", radius)
}

func GameObjectInterest(object Object, xField, yField, zField string) InterestManagementConfig {
	return objectInterestConfig(InterestGameObject, object, xField, yField, zField)
}

func GameObjectInterest2D(object Object, xField, yField string) InterestManagementConfig {
	return GameObjectInterest(object, xField, yField, "")
}

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

func withinInterestRadius(a, b interestPosition, radius float64) bool {
	dx := a.X - b.X
	dy := a.Y - b.Y

	if a.HasZ && b.HasZ {
		dz := a.Z - b.Z
		return (dx*dx + dy*dy + dz*dz) <= (radius * radius)
	}

	return (dx*dx + dy*dy) <= (radius * radius)
}

type interestState struct {
	enabled bool
	players map[string]InterestManagementConfig
	objects map[string]InterestManagementConfig
}

func newInterestManager() *InterestManager {
	return &InterestManager{
		players: map[string]InterestManagementConfig{},
		objects: map[string]InterestManagementConfig{},
	}
}

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

func interestObjectKey(objectType, objectID string) string {
	return normalizeObjectKey(objectType) + "/" + normalizeObjectKey(objectID)
}

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

func indirectValue(value reflect.Value) reflect.Value {
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer) {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}

	return value
}
