package network

import (
	"Enserva/objects"
	"testing"
)

func TestPlayerApplyInputBlocksOverlappingMove(t *testing.T) {
	rules := objects.DefaultPlayerRules(objects.NewWorldConfig(720, 405, 0, objects.Dimension2D))
	player := objects.NewPlayer("a", 340, 200)
	other := objects.NewPlayer("b", 361, 200)

	moved := player.ApplyInput(objects.MovementInput{X: 1, Y: 0}, 0.1, rules, []*objects.Player{other})

	if moved {
		t.Fatalf("expected collision move to be blocked")
	}
	if objects.PlayersCollide(player, other, objects.Dimension2D, playerRadius) {
		t.Fatalf("expected collision move to be blocked, got player x %.2f and other x %.2f", player.X, other.X)
	}
	if player.X != 340 {
		t.Fatalf("expected blocked player to stay at x 340, got %.2f", player.X)
	}
}

func TestPlayerApplyInputAllowsClearMove(t *testing.T) {
	rules := objects.DefaultPlayerRules(objects.NewWorldConfig(720, 405, 0, objects.Dimension2D))
	player := objects.NewPlayer("a", 200, 200)
	other := objects.NewPlayer("b", 361, 200)

	moved := player.ApplyInput(objects.MovementInput{X: 1, Y: 0}, 0.1, rules, []*objects.Player{other})

	if !moved {
		t.Fatalf("expected clear move to be allowed")
	}
	if player.X <= 200 {
		t.Fatalf("expected clear move to advance player, got x %.2f", player.X)
	}
}

func TestEnforcePlayerKeepsWholePlayerInsideWorld(t *testing.T) {
	rules := objects.DefaultPlayerRules(objects.NewWorldConfig(720, 405, 0, objects.Dimension2D))
	player := objects.NewPlayer("a", -100, 999)

	player.EnforceSecurity(rules)

	if player.X != playerRadius {
		t.Fatalf("expected left edge to clamp to radius %.2f, got %.2f", playerRadius, player.X)
	}
	if player.Y != 405-playerRadius {
		t.Fatalf("expected bottom edge to clamp to world minus radius %.2f, got %.2f", 405-playerRadius, player.Y)
	}

	player.UpdatePosition(999, -100)
	player.EnforceSecurity(rules)

	if player.X != 720-playerRadius {
		t.Fatalf("expected right edge to clamp to world minus radius %.2f, got %.2f", 720-playerRadius, player.X)
	}
	if player.Y != playerRadius {
		t.Fatalf("expected top edge to clamp to radius %.2f, got %.2f", playerRadius, player.Y)
	}
}

func TestApplyInputClampsToRadiusBounds(t *testing.T) {
	rules := objects.DefaultPlayerRules(objects.NewWorldConfig(720, 405, 0, objects.Dimension2D))
	player := objects.NewPlayer("a", playerRadius, 200)

	player.ApplyInput(objects.MovementInput{X: -1, Y: 0}, 1, rules, nil)

	if player.X != playerRadius {
		t.Fatalf("expected player to stay fully inside left edge at %.2f, got %.2f", playerRadius, player.X)
	}
}

func TestPlayerApplyInputSupports3D(t *testing.T) {
	world := objects.NewWorldConfig(720, 405, 180, objects.Dimension3D)
	rules := objects.DefaultPlayerRules(world)
	player := objects.NewPlayerInWorld("a", world, 200, 200, playerRadius)

	player.ApplyInput(objects.MovementInput{Z: 1}, 0.1, rules, nil)

	if player.Dimension != objects.Dimension3D {
		t.Fatalf("expected 3d player dimension, got %s", player.Dimension)
	}
	if player.Z <= playerRadius {
		t.Fatalf("expected 3d input to move on z axis, got %.2f", player.Z)
	}
}

func TestPlayerApplyInputBlocks3DObstacle(t *testing.T) {
	world := objects.NewWorldConfig(720, 405, 180, objects.Dimension3D)
	obstacle := objects.Obstacle{
		ID:     "test-obstacle",
		X:      100,
		Y:      100,
		Z:      10,
		Width:  40,
		Depth:  40,
		Height: 20,
	}
	rules := objects.DefaultPlayerRules(world)
	rules.Obstacles = []objects.Obstacle{obstacle}
	player := objects.NewPlayerInWorld("a", world, 100, 70, playerRadius)

	moved := player.ApplyInput(objects.MovementInput{Y: 1}, 0.1, rules, nil)

	if moved {
		t.Fatalf("expected obstacle move to be blocked")
	}
	if player.Y != 70 {
		t.Fatalf("expected blocked player to stay at y 70, got %.2f", player.Y)
	}
}

func TestPlayerSecurityClamps3DToWorldBounds(t *testing.T) {
	world := objects.NewWorldConfig(720, 405, 180, objects.Dimension3D)
	rules := objects.DefaultPlayerRules(world)
	player := objects.NewPlayerInWorld("a", world, -100, 999, 999)

	player.EnforceSecurity(rules)

	if player.X != playerRadius {
		t.Fatalf("expected x to clamp to radius %.2f, got %.2f", playerRadius, player.X)
	}
	if player.Y != 405-playerRadius {
		t.Fatalf("expected y to clamp to world edge %.2f, got %.2f", 405-playerRadius, player.Y)
	}
	if player.Z != 180-playerRadius {
		t.Fatalf("expected z to clamp to world edge %.2f, got %.2f", 180-playerRadius, player.Z)
	}
}
