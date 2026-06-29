package netobjects

import (
	"Enserva/network"
	"strings"
)

type Building struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Z          float64 `json:"z,omitempty"`
	Health     int     `json:"health"`
	Requests   uint64  `json:"requests"`
	Seconds    uint64  `json:"seconds"`
	LastClient string  `json:"lastClient,omitempty"`
}

type BuildingRequest struct {
	Kind   string  `json:"kind,omitempty"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Z      float64 `json:"z,omitempty"`
	Health int     `json:"health,omitempty"`
	Amount int     `json:"amount,omitempty"`
}

func NewBuilding(id string) *Building {
	return &Building{
		ID:     id,
		Kind:   "building",
		Health: 100,
	}
}

func BuildingFactory(ctx network.RequestContext) (network.Object, error) {
	return NewBuilding(ctx.Request.ObjectID), nil
}

func (building *Building) ObjectType() string {
	return "building"
}

func (building *Building) ObjectID() string {
	return building.ID
}

func (building *Building) Snapshot() any {
	return *building
}

func (building *Building) OnInit(ctx network.InitContext) {
	ctx.Runtime().Features().EnableInterestManagement(network.GameObjectInterest(building, "x", "y", "z"))
}

func (building *Building) OnFullTick(ctx network.TickContext) {
	building.Seconds++
}

func (building *Building) OnRequest(ctx network.RequestContext) error {
	var request BuildingRequest
	if err := ctx.Decode(&request); err != nil {
		return err
	}

	switch strings.ToLower(strings.TrimSpace(ctx.Request.Action)) {
	case "", "set", "place":
		if request.Kind != "" {
			building.Kind = request.Kind
		}
		if request.Health > 0 {
			building.Health = request.Health
		}
		building.X = request.X
		building.Y = request.Y
		building.Z = request.Z
	case "damage":
		building.Health -= request.Amount
		if building.Health < 0 {
			building.Health = 0
		}
	case "repair":
		building.Health += request.Amount
	}

	building.Requests++
	building.LastClient = ctx.ClientID
	return nil
}
