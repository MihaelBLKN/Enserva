package netobjects

import (
	"Enserva/network"
	"errors"
	"fmt"
	"strings"
)

const defaultPlayerSpeed = 180.0

var (
	ErrUnauthorizedPlayerClient = errors.New("client is not authorized for player")
	ErrUnsupportedPlayerAction  = errors.New("unsupported player action")
	ErrMissingPlayerRuntime     = errors.New("missing player runtime")
)

type PlayerAuthenticator struct {
	ID           string `json:"id"`
	NextPlayerID uint64 `json:"nextPlayerId"`
}

type Player struct {
	ID            string  `json:"id"`
	X             float64 `json:"x"`
	Y             float64 `json:"y"`
	Z             float64 `json:"z,omitempty"`
	VelocityX     float64 `json:"velocityX"`
	VelocityY     float64 `json:"velocityY"`
	VelocityZ     float64 `json:"velocityZ,omitempty"`
	Speed         float64 `json:"speed"`
	Requests      uint64  `json:"requests"`
	Seconds       uint64  `json:"seconds"`
	LastClient    string  `json:"lastClient,omitempty"`
	OwnerClientID string  `json:"-"`
}

type PlayerRequest struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Z     float64 `json:"z,omitempty"`
	Speed float64 `json:"speed,omitempty"`
}

func NewPlayerAuthenticator(id string) *PlayerAuthenticator {
	return &PlayerAuthenticator{ID: id}
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

func (authenticator *PlayerAuthenticator) ObjectType() string {
	return "player-auth"
}

func (authenticator *PlayerAuthenticator) ObjectID() string {
	return authenticator.ID
}

func (authenticator *PlayerAuthenticator) Snapshot() any {
	return nil
}

func (authenticator *PlayerAuthenticator) SnapshotVisible() bool {
	return false
}

func (authenticator *PlayerAuthenticator) OnAuthenticationAttempt(ctx network.AuthenticationContext) (string, error) {
	if ctx.Runtime == nil {
		return "", ErrMissingPlayerRuntime
	}

	authenticator.NextPlayerID++
	playerID := fmt.Sprintf("player-%d", authenticator.NextPlayerID)
	player := NewPlayer(playerID)
	player.AssignClient(playerID)

	if err := ctx.Runtime.RegisterObject(player); err != nil {
		return "", err
	}

	return playerID, nil
}

func (player *Player) AssignClient(clientID string) {
	player.OwnerClientID = strings.TrimSpace(clientID)
}

func (player *Player) SetSpeed(speed float64) {
	if speed > 0 {
		player.Speed = speed
	}
}

func (player *Player) Teleport(x, y, z float64) {
	player.X = x
	player.Y = y
	player.Z = z
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
	if player.OwnerClientID != "" && ctx.ClientID != player.OwnerClientID {
		return fmt.Errorf("%w: %s", ErrUnauthorizedPlayerClient, ctx.ClientID)
	}

	var request PlayerRequest
	if err := ctx.Decode(&request); err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(ctx.Request.Action)) {
	case "", "input", "move":
		player.VelocityX = clamp(request.X, -1, 1)
		player.VelocityY = clamp(request.Y, -1, 1)
		player.VelocityZ = clamp(request.Z, -1, 1)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedPlayerAction, ctx.Request.Action)
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
