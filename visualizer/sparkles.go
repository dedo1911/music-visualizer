package visualizer

import (
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ray is a single ray that starts from the center and goes toward the edge.
type ray struct {
	angle float64 // direction in radians
	life  float64 // 0..1
	hue   float64
	len   float64 // length relative to screen (0..1)
	width float32
}

// Sparkles generates starburst rays on hi-hats.
type Sparkles struct {
	pool []ray
}

func newSparkles() *Sparkles {
	return &Sparkles{}
}

func (s *Sparkles) burst(n, screenW, screenH int, hue, strength float64) {
	for i := 0; i < n; i++ {
		s.pool = append(s.pool, ray{
			angle: rand.Float64() * 2 * math.Pi,
			life:  0.7 + rand.Float64()*0.3,
			hue:   hue + rand.Float64()*50 - 25,
			len:   0.3 + rand.Float64()*0.5*strength,
			width: float32(0.5 + rand.Float64()*1.5*strength),
		})
	}
	if len(s.pool) > 500 {
		s.pool = s.pool[len(s.pool)-500:]
	}
}

func (s *Sparkles) update(speedFactor float64) {
	decay := 0.15 * speedFactor
	alive := s.pool[:0]
	for i := range s.pool {
		s.pool[i].life -= decay
		if s.pool[i].life > 0 {
			alive = append(alive, s.pool[i])
		}
	}
	s.pool = alive
}

func (s *Sparkles) draw(dst *ebiten.Image, cx, cy float64, screenH int) {
	maxR := float64(screenH) * 0.55

	for _, r := range s.pool {
		// The ray starts from 15% of max radius and extends to r.len
		r0 := maxR * 0.15
		r1 := maxR * r.len

		x0 := float32(cx + math.Cos(r.angle)*r0)
		y0 := float32(cy + math.Sin(r.angle)*r0)
		x1 := float32(cx + math.Cos(r.angle)*r1)
		y1 := float32(cy + math.Sin(r.angle)*r1)

		// Nearly white with slight tint
		c := hsvToRGB(r.hue, 0.25, 1.0)
		c.A = uint8(r.life * 220)

		vector.StrokeLine(dst, x0, y0, x1, y1, r.width*float32(r.life), c, true)
	}
}
