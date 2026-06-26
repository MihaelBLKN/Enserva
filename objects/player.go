package objects

import (
	"fmt"
	"math"
	"strings"
)

const (
	DefaultPlayerSpeed  = 180.0
	DefaultPlayerRadius = 10.0
	DefaultSpawnSpacing = 48.0
	DefaultWorldZ       = 180.0
)

type Dimension string

const (
	Dimension2D Dimension = "2d"
	Dimension3D Dimension = "3d"
)

type WorldConfig struct {
	X         float64    `json:"gameWorldX"`
	Y         float64    `json:"gameWorldY"`
	Z         float64    `json:"gameWorldZ,omitempty"`
	Dimension Dimension  `json:"dimension"`
	Obstacles []Obstacle `json:"obstacles,omitempty"`
}

type MovementInput struct {
	X float64
	Y float64
	Z float64
}

type PlayerRules struct {
	World     WorldConfig
	Speed     float64
	Radius    float64
	Obstacles []Obstacle
}

type Obstacle struct {
	ID     string  `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Z      float64 `json:"z"`
	Width  float64 `json:"width"`
	Depth  float64 `json:"depth"`
	Height float64 `json:"height"`
}

type Player struct {
	ID        string    `json:"id"`
	X         float64   `json:"x"`
	Y         float64   `json:"y"`
	Z         float64   `json:"z,omitempty"`
	Dimension Dimension `json:"dimension"`
}

func ParseDimension(value string) (Dimension, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "2", "2d":
		return Dimension2D, nil
	case "3", "3d":
		return Dimension3D, nil
	default:
		return "", fmt.Errorf("invalid player dimension %q (expected 2d or 3d)", value)
	}
}

func (dimension Dimension) Normalize() Dimension {
	if dimension == Dimension3D {
		return Dimension3D
	}

	return Dimension2D
}

func (dimension Dimension) Is3D() bool {
	return dimension.Normalize() == Dimension3D
}

func NewWorldConfig(x, y, z float64, dimension Dimension) WorldConfig {
	world := WorldConfig{
		X:         x,
		Y:         y,
		Z:         z,
		Dimension: dimension.Normalize(),
	}

	return world.Normalized()
}

func (world WorldConfig) Normalized() WorldConfig {
	world.Dimension = world.Dimension.Normalize()

	if !world.Dimension.Is3D() {
		world.Z = 0
		world.Obstacles = nil
		return world
	}

	if world.Z <= 0 {
		world.Z = DefaultWorldZ
	}
	if len(world.Obstacles) == 0 {
		world.Obstacles = DefaultObstacles(world)
	}

	return world
}

func DefaultPlayerRules(world WorldConfig) PlayerRules {
	world = world.Normalized()

	return PlayerRules{
		World:     world,
		Speed:     DefaultPlayerSpeed,
		Radius:    DefaultPlayerRadius,
		Obstacles: world.Obstacles,
	}
}

func (rules PlayerRules) Normalized() PlayerRules {
	rules.World = rules.World.Normalized()

	if rules.Speed <= 0 {
		rules.Speed = DefaultPlayerSpeed
	}
	if rules.Radius <= 0 {
		rules.Radius = DefaultPlayerRadius
	}
	if len(rules.Obstacles) == 0 {
		rules.Obstacles = rules.World.Obstacles
	}

	return rules
}

func NewPlayer(id string, x, y float64) *Player {
	return &Player{
		ID:        id,
		X:         x,
		Y:         y,
		Dimension: Dimension2D,
	}
}

func NewPlayerInWorld(id string, world WorldConfig, x, y, z float64) *Player {
	world = world.Normalized()
	if !world.Dimension.Is3D() {
		z = 0
	}

	player := &Player{
		ID:        id,
		X:         x,
		Y:         y,
		Z:         z,
		Dimension: world.Dimension,
	}
	player.EnforceSecurity(DefaultPlayerRules(world))

	return player
}

func SpawnPlayer(id string, world WorldConfig, playerIndex int) *Player {
	x, y, z := SpawnPosition(world, playerIndex)
	return NewPlayerInWorld(id, world, x, y, z)
}

func SpawnPosition(world WorldConfig, playerIndex int) (float64, float64, float64) {
	world = world.Normalized()

	z := 0.0
	if world.Dimension.Is3D() {
		z = DefaultPlayerRadius
	}

	if playerIndex == 0 {
		return world.X / 2, world.Y / 2, z
	}

	ring := (playerIndex + 5) / 6
	slot := (playerIndex - 1) % 6
	angle := (math.Pi * 2 / 6) * float64(slot)
	radius := DefaultSpawnSpacing * float64(ring)

	x := world.X/2 + math.Cos(angle)*radius
	y := world.Y/2 + math.Sin(angle)*radius

	return Clamp(x, DefaultPlayerRadius, world.X-DefaultPlayerRadius),
		Clamp(y, DefaultPlayerRadius, world.Y-DefaultPlayerRadius),
		z
}

func (p *Player) ApplyInput(input MovementInput, deltaSeconds float64, rules PlayerRules, others []*Player) bool {
	rules = rules.Normalized()
	normalized := NormalizeMovement(input, rules.World.Dimension)
	next := p.Clone()

	next.X += normalized.X * rules.Speed * deltaSeconds
	next.Y += normalized.Y * rules.Speed * deltaSeconds
	if rules.World.Dimension.Is3D() {
		next.Z += normalized.Z * rules.Speed * deltaSeconds
	}

	next.EnforceSecurity(rules)
	if CollidesWithAny(&next, others, rules.World.Dimension, rules.Radius) {
		return false
	}
	if CollidesWithAnyObstacle(&next, rules.Obstacles, rules.World.Dimension, rules.Radius) {
		return false
	}

	p.UpdateFrom(next)
	p.EnforceSecurity(rules)
	return true
}

func (p *Player) EnforceSecurity(rules PlayerRules) {
	rules = rules.Normalized()
	p.Dimension = rules.World.Dimension
	p.enforceWorldBounds(rules)

	if !rules.World.Dimension.Is3D() {
		p.Z = 0
	}
}

func (p *Player) UpdatePosition(x, y float64) {
	p.X = x
	p.Y = y

	if !p.Dimension.Is3D() {
		p.Z = 0
	}
}

func (p *Player) UpdatePosition3D(x, y, z float64) {
	p.X = x
	p.Y = y
	p.Z = z
	p.Dimension = Dimension3D
}

func (p *Player) UpdateFrom(data Player) {
	if data.ID != "" {
		p.ID = data.ID
	}
	if data.Dimension != "" {
		p.Dimension = data.Dimension.Normalize()
	}

	p.X = data.X
	p.Y = data.Y
	p.Z = data.Z

	if !p.Dimension.Is3D() {
		p.Z = 0
	}
}

func (p Player) Clone() Player {
	p.Dimension = p.Dimension.Normalize()
	if !p.Dimension.Is3D() {
		p.Z = 0
	}

	return p
}

func (p *Player) GetPosition() (float64, float64) {
	return p.X, p.Y
}

func (p *Player) GetPosition3D() (float64, float64, float64) {
	return p.X, p.Y, p.Z
}

func (p *Player) enforceWorldBounds(rules PlayerRules) {
	world := rules.World.Normalized()
	radius := rules.Radius

	p.X = Clamp(p.X, radius, world.X-radius)
	p.Y = Clamp(p.Y, radius, world.Y-radius)

	if world.Dimension.Is3D() {
		p.Z = Clamp(p.Z, radius, world.Z-radius)
	}
}

func NormalizeMovement(input MovementInput, dimension Dimension) MovementInput {
	x := Clamp(input.X, -1, 1)
	y := Clamp(input.Y, -1, 1)
	z := 0.0
	if dimension.Is3D() {
		z = Clamp(input.Z, -1, 1)
	}

	length := math.Hypot(x, y)
	if dimension.Is3D() {
		length = math.Sqrt(x*x + y*y + z*z)
	}
	if length <= 1 {
		return MovementInput{X: x, Y: y, Z: z}
	}

	return MovementInput{
		X: x / length,
		Y: y / length,
		Z: z / length,
	}
}

func CollidesWithAny(player *Player, others []*Player, dimension Dimension, radius float64) bool {
	for _, other := range others {
		if other == nil || other.ID == player.ID {
			continue
		}

		if PlayersCollide(player, other, dimension, radius) {
			return true
		}
	}

	return false
}

func PlayersCollide(playerA, playerB *Player, dimension Dimension, radius float64) bool {
	dx := playerA.X - playerB.X
	dy := playerA.Y - playerB.Y
	distance := math.Hypot(dx, dy)

	if dimension.Is3D() {
		dz := playerA.Z - playerB.Z
		distance = math.Sqrt(dx*dx + dy*dy + dz*dz)
	}

	return distance < radius*2
}

func DefaultObstacles(world WorldConfig) []Obstacle {
	if world.Z <= 0 {
		world.Z = DefaultWorldZ
	}

	return []Obstacle{
		{
			ID:     "relay-core",
			X:      world.X * 0.5,
			Y:      world.Y * 0.72,
			Z:      18,
			Width:  90,
			Depth:  42,
			Height: 36,
		},
		{
			ID:     "left-stack",
			X:      world.X * 0.2,
			Y:      world.Y * 0.42,
			Z:      22,
			Width:  54,
			Depth:  68,
			Height: 44,
		},
		{
			ID:     "right-wall",
			X:      world.X * 0.74,
			Y:      world.Y * 0.36,
			Z:      28,
			Width:  40,
			Depth:  112,
			Height: 56,
		},
		{
			ID:     "service-pillar",
			X:      world.X * 0.6,
			Y:      world.Y * 0.18,
			Z:      32,
			Width:  46,
			Depth:  46,
			Height: 64,
		},
	}
}

func CollidesWithAnyObstacle(player *Player, obstacles []Obstacle, dimension Dimension, radius float64) bool {
	if !dimension.Is3D() {
		return false
	}

	for _, obstacle := range obstacles {
		if PlayerCollidesWithObstacle(player, obstacle, radius) {
			return true
		}
	}

	return false
}

func PlayerCollidesWithObstacle(player *Player, obstacle Obstacle, radius float64) bool {
	minX := obstacle.X - obstacle.Width/2
	maxX := obstacle.X + obstacle.Width/2
	minY := obstacle.Y - obstacle.Depth/2
	maxY := obstacle.Y + obstacle.Depth/2
	minZ := obstacle.Z - obstacle.Height/2
	maxZ := obstacle.Z + obstacle.Height/2

	closestX := Clamp(player.X, minX, maxX)
	closestY := Clamp(player.Y, minY, maxY)
	closestZ := Clamp(player.Z, minZ, maxZ)
	dx := player.X - closestX
	dy := player.Y - closestY
	dz := player.Z - closestZ

	return dx*dx+dy*dy+dz*dz < radius*radius
}

func Clamp(value, min, max float64) float64 {
	if max < min {
		return min
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}

	return value
}
