package visualizer

import (
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type orb struct {
	x, y       float64 // normalized position 0..1
	baseRadius float64
	phase      float64 // phase offset for individual oscillation
	rotation   float64 // random rotation in radians
	rotSpeed   float64 // rotation speed
	driftX     float64 // drift speed
	driftY     float64
}

// Orbs are luminous spheres that float and pulse with the bass (60-200Hz).
type Orbs struct {
	pool []orb
}

func newOrbs(count int) *Orbs {
	o := &Orbs{pool: make([]orb, count)}
	for i := range o.pool {
		// Position only at edges: avoid the center (0.25-0.75)
		x, y := randomEdgePosition()
		o.pool[i] = orb{
			x:          x,
			y:          y,
			baseRadius: 3 + rand.Float64()*6,
			phase:      rand.Float64() * 2 * math.Pi,
			rotation:   rand.Float64() * 2 * math.Pi,
			rotSpeed:   (rand.Float64()*2 - 1) * 0.005,
			driftX:     (rand.Float64()*2 - 1) * 0.0001,
			driftY:     (rand.Float64()*2 - 1) * 0.00008,
		}
	}
	return o
}

// randomEdgePosition generates a position in the outer bands of the screen,
// avoiding the central square (0.25-0.75 on both axes).
func randomEdgePosition() (float64, float64) {
	for {
		x := 0.03 + rand.Float64()*0.94
		y := 0.03 + rand.Float64()*0.94
		// Accept only if at least one axis is in the outer band
		if x < 0.22 || x > 0.78 || y < 0.22 || y > 0.78 {
			return x, y
		}
	}
}

func (o *Orbs) kick(strength float64) {
	for i := range o.pool {
		ob := &o.pool[i]
		// New random direction and speed, proportional to kick strength
		ob.driftX = (rand.Float64()*2 - 1) * 0.0003 * (0.5 + strength)
		ob.driftY = (rand.Float64()*2 - 1) * 0.0003 * (0.5 + strength)
		// Jolt to rotation
		ob.rotSpeed = (rand.Float64()*2 - 1) * 0.012 * (0.5 + strength)
	}
}

func (o *Orbs) update(t float64) {
	for i := range o.pool {
		ob := &o.pool[i]
		ob.rotation += ob.rotSpeed
		ob.x += ob.driftX
		ob.y += ob.driftY
		// Bounce off edges and repel from center
		if ob.x < 0.02 || ob.x > 0.98 {
			ob.driftX = -ob.driftX
		}
		if ob.y < 0.02 || ob.y > 0.98 {
			ob.driftY = -ob.driftY
		}
		// If drifting toward the center, push outward
		if ob.x > 0.22 && ob.x < 0.78 && ob.y > 0.22 && ob.y < 0.78 {
			if ob.x < 0.5 {
				ob.driftX -= 0.00005
			} else {
				ob.driftX += 0.00005
			}
			if ob.y < 0.5 {
				ob.driftY -= 0.00005
			} else {
				ob.driftY += 0.00005
			}
		}
	}
}

func (o *Orbs) draw(dst *ebiten.Image, w, h int, bassEnergy, hue float64, samples []float32) {
	const steps = 48 // points to draw the outline

	for _, ob := range o.pool {
		pulse := bassEnergy * 1.0
		radius := ob.baseRadius * (0.7 + pulse*0.3)
		if radius < 2 {
			radius = 2
		}

		cx := ob.x * float64(w)
		cy := ob.y * float64(h)

		orbHue := hue + ob.phase*30
		brightness := 0.15 + bassEnergy*0.6
		if brightness > 1 {
			brightness = 1
		}

		// Gradient glow: concentric circles with decreasing alpha
		glowLayers := 3
		maxGlowR := radius * 3.0
		for l := glowLayers; l >= 1; l-- {
			t := float64(l) / float64(glowLayers)
			r := float32(maxGlowR * t)
			gc := hsvToRGB(orbHue, 0.5, brightness*0.25)
			gc.A = uint8(float64(1-t) * bassEnergy * 25)
			vector.DrawFilledCircle(dst, float32(cx), float32(cy), r, gc, false)
		}

		// Waveform-distorted edge: the radius at each angle
		// is modulated by the corresponding audio sample
		c := hsvToRGB(orbHue, 0.35, brightness*0.9)
		c.A = uint8(30 + bassEnergy*120)
		lineW := float32(1.0 + bassEnergy*1.5)

		sLen := len(samples)
		if sLen == 0 {
			continue
		}

		var prevX, prevY float32
		var firstX, firstY float32

		for i := 0; i <= steps; i++ {
			// Angle with individual rotation
			angle := ob.rotation + float64(i)/float64(steps)*2*math.Pi

			// Waveform sample mapped to this point on the circle
			sIdx := int(float64(i) / float64(steps) * float64(sLen))
			if sIdx >= sLen {
				sIdx = sLen - 1
			}
			waveform := float64(samples[sIdx])

			// The radius oscillates with the waveform
			r := radius + waveform*radius*1.8
			if r < 1 {
				r = 1
			}

			px := float32(cx + math.Cos(angle)*r)
			py := float32(cy + math.Sin(angle)*r)

			if i == 0 {
				firstX, firstY = px, py
			} else {
				vector.StrokeLine(dst, prevX, prevY, px, py, lineW, c, false)
			}
			prevX, prevY = px, py
		}
		// Close the circle
		vector.StrokeLine(dst, prevX, prevY, firstX, firstY, lineW, c, false)
	}
}
