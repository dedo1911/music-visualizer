package visualizer

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type Torus struct {
	rotX, rotY, rotZ float64 // current rotation in degrees
	distort          float64 // beat-triggered distortion amount
}

func newTorus() *Torus {
	return &Torus{}
}

func (t *Torus) update(speedFactor float64, isKick bool, kickStrength, buildupIntensity float64) {
	if isKick {
		t.distort += kickStrength * 0.18
	}
	// Buildup adds continuous increasing distortion
	t.distort += buildupIntensity * 0.02
	t.distort *= 0.86

	// Buildup accelerates the rotation
	sf := speedFactor * (1 + buildupIntensity*0.8)
	t.rotY += 0.55 * sf
	t.rotX += 0.20 * sf
	t.rotZ += 0.07 * sf
}

func (t *Torus) draw(dst *ebiten.Image, cx, cy, scale, hue, energy float64) {
	const (
		majorR = 0.38
		minorR = 0.15
		rings  = 28
		segs   = 18
		fov    = 2.5
	)

	rxR := t.rotX * math.Pi / 180
	ryR := t.rotY * math.Pi / 180
	rzR := t.rotZ * math.Pi / 180

	cosX, sinX := math.Cos(rxR), math.Sin(rxR)
	cosY, sinY := math.Cos(ryR), math.Sin(ryR)
	cosZ, sinZ := math.Cos(rzR), math.Sin(rzR)

	rotate := func(x, y, z float64) (float64, float64, float64) {
		// X axis
		y, z = y*cosX-z*sinX, y*sinX+z*cosX
		// Y axis
		x, z = x*cosY+z*sinY, -x*sinY+z*cosY
		// Z axis
		x, y = x*cosZ-y*sinZ, x*sinZ+y*cosZ
		return x, y, z
	}

	project := func(x, y, z float64) (float32, float32, bool) {
		if z+fov < 0.01 {
			return 0, 0, false
		}
		s := fov / (z + fov)
		return float32(cx + x*s*scale), float32(cy + y*s*scale), true
	}

	pt := func(u, v float64) (float32, float32, bool) {
		wave := t.distort * math.Sin(u*3+v*2+t.rotY*0.05)
		r := minorR + wave
		x := (majorR + r*math.Cos(v)) * math.Cos(u)
		y := (majorR + r*math.Cos(v)) * math.Sin(u)
		z := r * math.Sin(v)
		rx, ry, rz := rotate(x, y, z)
		return project(rx, ry, rz)
	}

	lineW := float32(1.1 + energy*2.0)

	for ri := 0; ri < rings; ri++ {
		u0 := 2 * math.Pi * float64(ri) / rings
		u1 := 2 * math.Pi * float64(ri+1) / rings
		frac := float64(ri) / rings

		for si := 0; si < segs; si++ {
			v0 := 2 * math.Pi * float64(si) / segs
			v1 := 2 * math.Pi * float64(si+1) / segs

			x00, y00, ok00 := pt(u0, v0)
			x10, y10, ok10 := pt(u1, v0)
			x01, y01, ok01 := pt(u0, v1)

			c := hsvToRGB(hue+frac*120, 0.85, 0.8+energy*0.2)
			c.A = 210

			if ok00 && ok10 {
				vector.StrokeLine(dst, x00, y00, x10, y10, lineW, c, false)
			}
			if ok00 && ok01 {
				vector.StrokeLine(dst, x00, y00, x01, y01, lineW, c, false)
			}
		}
	}
}
