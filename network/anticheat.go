package network

import "Enserva/objects"

type AntiCheat struct {
	world WorldConfig
	rules objects.PlayerRules
}

func NewAntiCheat(world WorldConfig) *AntiCheat {
	rules := objects.DefaultPlayerRules(world)

	return &AntiCheat{
		world: rules.World,
		rules: rules,
	}
}

func (antiCheat *AntiCheat) EnforcePlayer(player *objects.Player) {
	player.EnforceSecurity(antiCheat.rules)
}

func clamp(value, min, max float64) float64 {
	return objects.Clamp(value, min, max)
}
