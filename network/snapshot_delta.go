package network

import (
	"encoding/json"
	"reflect"
	"sort"
)

// BuildSnapshotDelta compares a previous visible snapshot with the current
// visible snapshot and returns the spawned, changed, and despawned objects.
func BuildSnapshotDelta(previous, current SnapshotData) SnapshotDelta {
	delta := SnapshotDelta{
		Spawned: SnapshotData{},
		Changed: SnapshotData{},
	}

	for objectType, currentObjects := range current {
		previousObjects := previous[objectType]
		for objectID, currentSnapshot := range currentObjects {
			previousSnapshot, existed := previousObjects[objectID]
			if !existed {
				addSnapshotObject(delta.Spawned, objectType, objectID, currentSnapshot)
				continue
			}
			if !snapshotValuesEqual(previousSnapshot, currentSnapshot) {
				addSnapshotObject(delta.Changed, objectType, objectID, currentSnapshot)
			}
		}
	}

	for objectType, previousObjects := range previous {
		currentObjects := current[objectType]
		for objectID := range previousObjects {
			if _, ok := currentObjects[objectID]; ok {
				continue
			}
			delta.Despawned = append(delta.Despawned, SnapshotObjectRef{
				ObjectType: objectType,
				ObjectID:   objectID,
			})
		}
	}
	sort.Slice(delta.Despawned, func(left, right int) bool {
		if delta.Despawned[left].ObjectType == delta.Despawned[right].ObjectType {
			return delta.Despawned[left].ObjectID < delta.Despawned[right].ObjectID
		}
		return delta.Despawned[left].ObjectType < delta.Despawned[right].ObjectType
	})

	return delta
}

// CloneSnapshotData returns a detached JSON-shaped copy suitable for storing
// as a server-side delta baseline.
func CloneSnapshotData(snapshot SnapshotData) SnapshotData {
	if snapshot == nil {
		return SnapshotData{}
	}

	payload, err := json.Marshal(snapshot)
	if err == nil {
		var cloned SnapshotData
		if err := json.Unmarshal(payload, &cloned); err == nil && cloned != nil {
			return cloned
		}
	}

	cloned := SnapshotData{}
	for objectType, objectsByID := range snapshot {
		cloned[objectType] = map[string]any{}
		for objectID, value := range objectsByID {
			cloned[objectType][objectID] = value
		}
	}
	return cloned
}

func snapshotValuesEqual(left, right any) bool {
	leftPayload, leftErr := json.Marshal(left)
	rightPayload, rightErr := json.Marshal(right)
	if leftErr == nil && rightErr == nil {
		return string(leftPayload) == string(rightPayload)
	}

	return reflect.DeepEqual(left, right)
}
