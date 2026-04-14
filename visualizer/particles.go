package visualizer

import (
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type particle struct {
	x, y, z    float64
	vx, vy, vz float64
	life       float64
	hue        float64
	size       float64
}

type Particles struct {
	pool []particle
	maxN int
}

func newParticles(maxN int) *Particles {
	return &Particles{maxN: maxN}
}

func (ps *Particles) burst(n int, hue, strength float64) {
	for i := 0; i < n; i++ {
		speed := (0.012 + rand.Float64()*0.028) * (0.5 + strength*0.5)
		theta := rand.Float64() * 2 * math.Pi
		phi := math.Acos(2*rand.Float64() - 1)
		ps.pool = append(ps.pool, particle{
			vx:   speed * math.Sin(phi) * math.Cos(theta),
			vy:   speed * math.Sin(phi) * math.Sin(theta),
			vz:   speed * math.Cos(phi),
			life: 1.0,
			hue:  hue + rand.Float64()*90 - 45,
			size: 2 + rand.Float64()*5,
		})
	}
	if len(ps.pool) > ps.maxN {
		ps.pool = ps.pool[len(ps.pool)-ps.maxN:]
	}
}

func (ps *Particles) update(speedFactor float64) {
	lifeDecay := 0.013 * speedFactor
	alive := ps.pool[:0]
	for i := range ps.pool {
		p := &ps.pool[i]
		p.x += p.vx * speedFactor
		p.y += p.vy * speedFactor
		p.z += p.vz * speedFactor
		p.vx *= 0.97
		p.vy *= 0.97
		p.vz *= 0.97
		p.life -= lifeDecay
		if p.life > 0 {
			alive = append(alive, *p)
		}
	}
	ps.pool = alive
}

func (ps *Particles) draw(dst *ebiten.Image, cx, cy, scale float64) {
	const fov = 2.5
	for _, p := range ps.pool {
		if p.z+fov < 0.01 {
			continue
		}
		s := fov / (p.z + fov)
		sx := float32(cx + p.x*s*scale)
		sy := float32(cy + p.y*s*scale)
		size := float32(p.size * p.life * s)
		if size < 0.5 {
			size = 0.5
		}
		c := hsvToRGB(p.hue, 1.0, 1.0)
		c.A = uint8(p.life * 255)
		vector.DrawFilledCircle(dst, sx, sy, size, c, false)
	}
}
