package visualizer

import (
	"math"
	"math/rand"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type orb struct {
	x, y       float64 // posizione normalizzata 0..1
	baseRadius float64
	phase      float64 // offset fase per oscillazione individuale
	rotation   float64 // rotazione casuale in radianti
	rotSpeed   float64 // velocità di rotazione
	driftX     float64 // velocità drift
	driftY     float64
}

// Orbs sono sfere luminose che fluttuano e pulsano col basso (60-200Hz).
type Orbs struct {
	pool []orb
}

func newOrbs(count int) *Orbs {
	o := &Orbs{pool: make([]orb, count)}
	for i := range o.pool {
		// Posiziona solo ai bordi: evita il centro (0.25-0.75)
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

// randomEdgePosition genera una posizione nelle fasce esterne dello schermo,
// evitando il quadrato centrale (0.25-0.75 su entrambi gli assi).
func randomEdgePosition() (float64, float64) {
	for {
		x := 0.03 + rand.Float64()*0.94
		y := 0.03 + rand.Float64()*0.94
		// Accetta solo se almeno un asse è nella fascia esterna
		if x < 0.22 || x > 0.78 || y < 0.22 || y > 0.78 {
			return x, y
		}
	}
}

func (o *Orbs) kick(strength float64) {
	for i := range o.pool {
		ob := &o.pool[i]
		// Nuova direzione e velocità casuale, proporzionale alla forza del kick
		ob.driftX = (rand.Float64()*2 - 1) * 0.0003 * (0.5 + strength)
		ob.driftY = (rand.Float64()*2 - 1) * 0.0003 * (0.5 + strength)
		// Scossa alla rotazione
		ob.rotSpeed = (rand.Float64()*2 - 1) * 0.012 * (0.5 + strength)
	}
}

func (o *Orbs) update(t float64) {
	for i := range o.pool {
		ob := &o.pool[i]
		ob.rotation += ob.rotSpeed
		ob.x += ob.driftX
		ob.y += ob.driftY
		// Rimbalza ai bordi e respingi dal centro
		if ob.x < 0.02 || ob.x > 0.98 {
			ob.driftX = -ob.driftX
		}
		if ob.y < 0.02 || ob.y > 0.98 {
			ob.driftY = -ob.driftY
		}
		// Se deriva verso il centro, spingi fuori
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
	const steps = 48 // punti per disegnare il contorno

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

		// Glow sfumato: cerchi concentrici con alpha decrescente
		glowLayers := 3
		maxGlowR := radius * 3.0
		for l := glowLayers; l >= 1; l-- {
			t := float64(l) / float64(glowLayers)
			r := float32(maxGlowR * t)
			gc := hsvToRGB(orbHue, 0.5, brightness*0.25)
			gc.A = uint8(float64(1-t) * bassEnergy * 25)
			vector.DrawFilledCircle(dst, float32(cx), float32(cy), r, gc, false)
		}

		// Bordo distorto dalla waveform: il raggio ad ogni angolo
		// è modulato dal campione audio corrispondente
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
			// Angolo con rotazione individuale
			angle := ob.rotation + float64(i)/float64(steps)*2*math.Pi

			// Sample della waveform mappato su questo punto del cerchio
			sIdx := int(float64(i) / float64(steps) * float64(sLen))
			if sIdx >= sLen {
				sIdx = sLen - 1
			}
			waveform := float64(samples[sIdx])

			// Il raggio oscilla con la waveform
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
		// Chiudi il cerchio
		vector.StrokeLine(dst, prevX, prevY, firstX, firstY, lineW, c, false)
	}
}
