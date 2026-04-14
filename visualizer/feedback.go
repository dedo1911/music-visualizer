package visualizer

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Feedback implements the iconic MilkDrop zoom+rotation trail effect.
// Each frame the previous frame is drawn into the new one, slightly zoomed in,
// slightly rotated, and darkened — creating hypnotic trails.
type Feedback struct {
	bufs  [2]*ebiten.Image
	cur   int
	angle float64 // accumulated rotation in degrees
}

func newFeedback(w, h int) *Feedback {
	f := &Feedback{}
	f.bufs[0] = ebiten.NewImage(w, h)
	f.bufs[1] = ebiten.NewImage(w, h)
	return f
}

func (f *Feedback) resize(w, h int) {
	for i := range f.bufs {
		bw := f.bufs[i].Bounds().Dx()
		bh := f.bufs[i].Bounds().Dy()
		if bw != w || bh != h {
			f.bufs[i].Dispose()
			f.bufs[i] = ebiten.NewImage(w, h)
		}
	}
}

// advance draws the previous buffer into the new one with warp effect,
// returns the new canvas ready for drawing.
func (f *Feedback) advance(rotDelta, energy float64) *ebiten.Image {
	prev := f.bufs[f.cur]
	f.cur = 1 - f.cur
	curr := f.bufs[f.cur]
	curr.Clear()

	f.angle = math.Mod(f.angle+rotDelta, 360)

	w := float64(curr.Bounds().Dx())
	h := float64(curr.Bounds().Dy())
	zoom := 1.0025 + energy*0.004

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-w/2, -h/2)
	op.GeoM.Scale(zoom, zoom)
	op.GeoM.Rotate(f.angle * math.Pi / 180)
	op.GeoM.Translate(w/2, h/2)
	op.ColorScale.Scale(0.75, 0.75, 0.78, 1.0)

	curr.DrawImage(prev, op)
	return curr
}
