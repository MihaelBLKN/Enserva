package network

import "Enserva/objects"

const (
	simulationTickRate = 128
	snapshotRate       = 20
	playerSpeed        = objects.DefaultPlayerSpeed
	playerRadius       = objects.DefaultPlayerRadius
	clientTimeout      = 5
	spawnSpacing       = objects.DefaultSpawnSpacing
)

type WorldConfig = objects.WorldConfig
type PlayerDimension = objects.Dimension

type InputMessage struct {
	Sequence uint64  `json:"seq"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Z        float64 `json:"z,omitempty"`
}

type SnapshotMessage struct {
	Type         string                    `json:"type"`
	SelfID       string                    `json:"selfId"`
	Tick         uint64                    `json:"tick"`
	LastSequence uint64                    `json:"lastSeq"`
	World        WorldConfig               `json:"world"`
	Players      map[string]objects.Player `json:"players"`
}

func (input InputMessage) Movement() objects.MovementInput {
	return objects.MovementInput{
		X: input.X,
		Y: input.Y,
		Z: input.Z,
	}
}
