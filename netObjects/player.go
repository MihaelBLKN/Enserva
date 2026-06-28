package netobjects

import (
	"Enserva/network"
	"strings"
)

const defaultPlayerSpeed = 180.0

type Player struct {
	ID         string  `json:"id"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Z          float64 `json:"z,omitempty"`
	VelocityX  float64 `json:"velocityX"`
	VelocityY  float64 `json:"velocityY"`
	VelocityZ  float64 `json:"velocityZ,omitempty"`
	Speed      float64 `json:"speed"`
	Requests   uint64  `json:"requests"`
	Seconds    uint64  `json:"seconds"`
	LastClient string  `json:"lastClient,omitempty"`
}

type PlayerRequest struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Z     float64 `json:"z,omitempty"`
	Speed float64 `json:"speed,omitempty"`
}

func NewPlayer(id string) *Player {
	return &Player{
		ID:    id,
		Speed: defaultPlayerSpeed,
	}
}

func PlayerFactory(ctx network.RequestContext) (network.Object, error) {
	return NewPlayer(ctx.Request.ObjectID), nil
}

func (player *Player) ObjectType() string {
	return "player"
}

func (player *Player) ObjectID() string {
	return player.ID
}

func (player *Player) Snapshot() any {
	return *player
}

func (player *Player) OnTick(ctx network.TickContext) {
	player.X += player.VelocityX * player.Speed * ctx.DeltaSeconds
	player.Y += player.VelocityY * player.Speed * ctx.DeltaSeconds
	player.Z += player.VelocityZ * player.Speed * ctx.DeltaSeconds
}

func (player *Player) OnFullTick(ctx network.TickContext) {
	player.Seconds++
}

func (player *Player) OnRequest(ctx network.RequestContext) error {
	var request PlayerRequest
	if err := ctx.Decode(&request); err != nil {
		return err
	}

	if request.Speed > 0 {
		player.Speed = request.Speed
	}

	switch strings.ToLower(strings.TrimSpace(ctx.Request.Action)) {
	case "", "input", "move":
		player.VelocityX = clamp(request.X, -1, 1)
		player.VelocityY = clamp(request.Y, -1, 1)
		player.VelocityZ = clamp(request.Z, -1, 1)
	case "teleport", "set":
		player.X = request.X
		player.Y = request.Y
		player.Z = request.Z
	}

	player.Requests++
	player.LastClient = ctx.ClientID
	return nil
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}

	return value
}
