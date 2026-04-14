package visualizer

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// oversize: the plasma is rendered 1.4x larger than the screen
// so the edges stay out of view even after the feedback warp rotation.
const oversize = 2.0

// Plasma draws an animated color field with sinusoidal waves.
// Rendered at low resolution and scaled up for a psychedelic look.
type Plasma struct {
	img    *ebiten.Image
	pixels []byte
	pw, ph int
	sw, sh int // actual screen dimensions
	t      float64
}

func newPlasma() *Plasma {
	return &Plasma{}
}

func (p *Plasma) resize(screenW, screenH int) {
	// Plasma size = screen * oversize, then /8 for low resolution
	ow := int(float64(screenW) * oversize)
	oh := int(float64(screenH) * oversize)
	pw := ow / 14
	ph := oh / 14
	if pw == p.pw && ph == p.ph {
		return
	}
	p.pw = pw
	p.ph = ph
	p.sw = screenW
	p.sh = screenH
	if p.img != nil {
		p.img.Dispose()
	}
	p.img = ebiten.NewImage(pw, ph)
	p.pixels = make([]byte, pw*ph*4)
}

func (p *Plasma) update(energy, hue float64) {
	p.t += 0.008 + energy*0.018
}

func (p *Plasma) draw(dst *ebiten.Image, energy, hue float64) {
	if p.img == nil {
		return
	}
	t := p.t
	fw := float64(p.pw)
	fh := float64(p.ph)

	for y := 0; y < p.ph; y++ {
		for x := 0; x < p.pw; x++ {
			fx := float64(x) / fw
			fy := float64(y) / fh

			v := math.Sin(fx*5+t) +
				math.Sin(fy*4+t*0.71) +
				math.Sin((fx+fy)*3+t*0.53) +
				math.Sin(math.Sqrt(fx*fx+fy*fy)*7+t*1.1)
			v /= 4

			h := hue + v*50
			brightness := 0.04 + math.Abs(v)*0.06 + energy*0.03
			c := hsvToRGB(h, 0.85, brightness)

			// Radial vignette: fades to 0 at edges so the feedback
			// rotation never shows a hard cut
			dx := (fx - 0.5) * 2 // -1..1
			dy := (fy - 0.5) * 2
			dist := math.Sqrt(dx*dx + dy*dy) // 0 at center, ~1.41 at corners
			// Fade: full up to dist=0.6, fades to 0 at dist=1.0
			fade := 1.0 - math.Max(0, (dist-0.6)/0.4)
			if fade < 0 {
				fade = 0
			}

			idx := (y*p.pw + x) * 4
			p.pixels[idx] = c.R
			p.pixels[idx+1] = c.G
			p.pixels[idx+2] = c.B
			p.pixels[idx+3] = uint8(150 * fade)
		}
	}
	p.img.WritePixels(p.pixels)

	// Scale to oversized target and center on screen
	dw := float64(p.sw) * oversize
	dh := float64(p.sh) * oversize
	offsetX := -float64(p.sw) * (oversize - 1) / 2
	offsetY := -float64(p.sh) * (oversize - 1) / 2

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(dw/float64(p.pw), dh/float64(p.ph))
	op.GeoM.Translate(offsetX, offsetY)
	op.Blend = ebiten.BlendSourceOver
	dst.DrawImage(p.img, op)
}
