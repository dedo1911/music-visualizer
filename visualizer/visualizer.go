package visualizer

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"
	"time"

	"github.com/dedo1911/music-visualizer/audio"
	"github.com/dedo1911/music-visualizer/beat"
	"github.com/dedo1911/music-visualizer/dsp"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	NumBands    = 128
	SmoothAlpha = 0.85
	PeakDecay   = 0.97
	MinDB       = -60.0

	agcDecay    = 0.999
	agcMinLevel = 0.001
)

// shockwave is a ring that expands toward the edges on kick.
type shockwave struct {
	radius float64
	alpha  float64
	hue    float64
}

type Visualizer struct {
	capture *audio.Capture
	beat    *beat.Detector

	bands   []float32
	peaks   []float32
	agcPeak float32

	hue        float64
	energy     float64
	rotDelta   float64
	ringPulse  float64
	shockwaves []shockwave
	chromaShift float64 // R/B offset in pixels, jumps on kick and decays
	shakeX      float64 // screen shake X (buildup)
	shakeY      float64 // screen shake Y (buildup)
	showDebug   bool
	frameCount  int

	feedback  *Feedback
	torus     *Torus
	particles *Particles
	plasma    *Plasma
	sparkles  *Sparkles
	orbs      *Orbs
	bassEnergy        float64      // 60-200Hz energy for the orbs
	bassHistory       [480]float64 // ~8s at 60fps
	mixEnergyHistory  [480]float64
	hihatEnergyHist   [480]float64
	historyIdx        int
	rawSamples        []float32
	keyDet    *dsp.KeyDetector

	width, height int
}

func New(cap *audio.Capture, width, height int) *Visualizer {
	return &Visualizer{
		capture:   cap,
		beat:      &beat.Detector{},
		bands:     make([]float32, NumBands),
		peaks:     make([]float32, NumBands),
		width:     width,
		height:    height,
		torus:     newTorus(),
		particles: newParticles(1200),
		plasma:   newPlasma(),
		sparkles: newSparkles(),
		orbs:     newOrbs(18),
		keyDet:   dsp.NewKeyDetector(),
		rotDelta: 0.06,
	}
}

func (v *Visualizer) Update() error {
	if ebiten.IsKeyPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF11) {
		ebiten.SetFullscreen(!ebiten.IsFullscreen())
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		v.showDebug = !v.showDebug
	}

	// --- Audio processing ---
	samples := v.capture.GetSamples()
	v.rawSamples = samples
	rawBands := dsp.Spectrum(samples, NumBands)

	// Visual AGC
	var currentMax float32
	for _, m := range rawBands {
		if m > currentMax {
			currentMax = m
		}
	}
	if currentMax > v.agcPeak {
		v.agcPeak = currentMax
	} else {
		v.agcPeak *= agcDecay
	}
	if v.agcPeak < agcMinLevel {
		v.agcPeak = agcMinLevel
	}

	// Beat detector on raw data (has its own internal AGC)
	v.beat.Update(rawBands)

	// Key detector: uses the long buffer (16384 samples, ~2.7Hz resolution)
	// and runs every 15 frames (~4 Hz) to save CPU on the long FFT
	v.frameCount++
	if v.frameCount%120 == 0 {
		keySamples := v.capture.GetKeySamples()
		v.keyDet.Update(keySamples)
	}

	for i := range v.bands {
		mag := rawBands[i] / v.agcPeak
		var normalized float32
		if mag > 0 {
			db := 20.0 * math.Log10(float64(mag))
			normalized = float32((db - MinDB) / (-MinDB))
		}
		if normalized < 0 {
			normalized = 0
		}
		if normalized > 1 {
			normalized = 1
		}
		v.bands[i] = v.bands[i]*(1-SmoothAlpha) + normalized*SmoothAlpha
		if v.bands[i] > v.peaks[i] {
			v.peaks[i] = v.bands[i]
		} else {
			v.peaks[i] *= PeakDecay
		}
	}

	var totalEnergy float32
	for i := 8; i < NumBands; i++ {
		totalEnergy += v.bands[i]
	}
	v.energy = float64(totalEnergy) / float64(NumBands-8)

	// --- Visual state ---
	sf := v.beat.SpeedFactor()

	// Hue driven by key: oscillates around the key color
	// with amplitude inversely proportional to confidence.
	// High confidence → palette coherent with the key.
	// Low confidence (transition, silence) → freer colors.
	keyHue := v.keyDet.BaseHue()
	conf := v.keyDet.Confidence
	oscillation := 0.08 + sf*0.18 // oscillation speed scaled with BPM
	v.hue += oscillation

	// Pull the hue toward the key color: stronger with high confidence
	if conf > 0.5 {
		pull := (conf - 0.5) * 0.06 // 0..0.03 attraction force
		diff := keyHue - math.Mod(v.hue, 360)
		// Take the shortest path around the circle
		if diff > 180 {
			diff -= 360
		} else if diff < -180 {
			diff += 360
		}
		v.hue += diff * pull
	}

	v.rotDelta = 0.01 + v.energy*0.03 + v.beat.BassEnergy*0.02 + sf*0.015
	if v.beat.IsKick {
		v.rotDelta += v.beat.KickStrength * 0.6
	}

	if v.beat.IsKick {
		v.ringPulse = 0.5 + v.beat.KickStrength*0.5
		// Chromatic aberration: offset proportional to kick strength
		v.chromaShift = 6 + v.beat.KickStrength*18
	}
	// Decay scaled with BPM: at faster tempo, everything resolves quicker
	// sf ranges from 0.6 (124 BPM) to 2.2 (155 BPM)
	ringDecay := 0.84 - (sf-1.0)*0.04  // 0.84 → 0.79 at high BPM
	chromaDecay := 0.85 - (sf-1.0)*0.05 // 0.85 → 0.79 at high BPM
	v.ringPulse *= ringDecay
	v.chromaShift *= chromaDecay

	// Shockwaves
	if v.beat.IsKick {
		baseRadius := float64(v.height)*0.25 + v.ringPulse*float64(v.height)*0.08
		v.shockwaves = append(v.shockwaves, shockwave{
			radius: baseRadius,
			alpha:  v.beat.KickStrength,
			hue:    v.hue,
		})
		if len(v.shockwaves) > 8 {
			v.shockwaves = v.shockwaves[len(v.shockwaves)-8:]
		}
	}
	// Shockwave: expansion and fade scaled with BPM
	swSpeed := 0.018 + (sf-1.0)*0.008  // faster at high BPM
	swDecay := 0.88 - (sf-1.0)*0.04    // faster fade at high BPM
	alive := v.shockwaves[:0]
	for i := range v.shockwaves {
		sw := &v.shockwaves[i]
		sw.radius += float64(v.height) * swSpeed
		sw.alpha *= swDecay
		if sw.alpha > 0.01 {
			alive = append(alive, *sw)
		}
	}
	v.shockwaves = alive

	// Update subsystems
	// Bass energy 60-200Hz (bands 20-39) for the orbs
	var bassSum float32
	for i := 20; i < 39 && i < NumBands; i++ {
		bassSum += v.bands[i]
	}
	v.bassEnergy = float64(bassSum) / 19.0
	idx := v.historyIdx % len(v.bassHistory)
	v.bassHistory[idx] = v.bassEnergy
	v.mixEnergyHistory[idx] = v.energy
	v.hihatEnergyHist[idx] = v.beat.HihatEnergy
	v.historyIdx++

	v.plasma.update(v.energy+v.beat.HihatEnergy*0.5+sf*0.3, v.hue)
	v.orbs.update(v.hue)
	v.torus.update(sf, v.beat.IsKick, v.beat.KickStrength, v.beat.BuildupIntensity)
	v.particles.update(sf)
	v.sparkles.update(sf)
	if v.beat.IsKick {
		v.particles.burst(180, v.hue, v.beat.KickStrength)
		v.orbs.kick(v.beat.KickStrength)
	}
	bi := v.beat.BuildupIntensity
	if v.beat.IsHihat {
		// More rays during buildup: from 12 to 50
		rayCount := int(12 + bi*38)
		v.sparkles.burst(rayCount, v.width, v.height, v.hue+180, v.beat.HihatStrength+bi*0.5)
	}
	// Screen shake: buildup + kick impulse
	kickShake := 0.0
	if v.beat.IsKick && v.beat.KickStrength > 0.5 {
		kickShake = (v.beat.KickStrength - 0.5) * 2 // 0..1 only for strong kicks
	}
	totalShake := bi*55 + kickShake*15 // buildup dominates, kick adds a jolt
	v.shakeX = (rand.Float64()*2 - 1) * totalShake
	v.shakeY = (rand.Float64()*2 - 1) * totalShake * 0.7

	return nil
}

func (v *Visualizer) Draw(screen *ebiten.Image) {
	if v.feedback == nil {
		v.feedback = newFeedback(v.width, v.height)
	} else {
		v.feedback.resize(v.width, v.height)
	}
	v.plasma.resize(v.width, v.height)

	// Buildup increases feedback zoom and rotation → tunnel rush effect
	feedbackEnergy := v.energy + v.beat.BuildupIntensity*0.6
	canvas := v.feedback.advance(v.rotDelta+v.beat.BuildupIntensity*0.05, feedbackEnergy)

	v.plasma.draw(canvas, v.energy, v.hue)

	cx := float64(v.width) / 2
	cy := float64(v.height) / 2
	scale := float64(v.height) * 0.44

	v.torus.draw(canvas, cx, cy, scale, v.hue, v.energy)
	v.particles.draw(canvas, cx, cy, scale)

	baseRingRadius := float64(v.height) * 0.25
	ringRadius := baseRingRadius + v.ringPulse*float64(v.height)*0.08
	v.drawRing(canvas, cx, cy, ringRadius)
	v.drawShockwaves(canvas, cx, cy)

	// Composite canvas onto screen (normal SourceOver)
	if v.chromaShift > 0.5 {
		v.drawChromaAberration(screen, canvas)
	} else if math.Abs(v.shakeX) > 0.5 || math.Abs(v.shakeY) > 0.5 {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(v.shakeX, v.shakeY)
		screen.DrawImage(canvas, op)
	} else {
		screen.DrawImage(canvas, nil)
	}

	// Glitch strips during buildup: horizontally offset slices
	if v.beat.BuildupIntensity > 0.05 {
		v.drawGlitch(screen, canvas, v.beat.BuildupIntensity)
	}

	v.orbs.draw(screen, v.width, v.height, v.bassEnergy, v.hue, v.rawSamples)
	v.sparkles.draw(screen, float64(v.width)/2, float64(v.height)/2, v.height)

	if v.showDebug {
		v.drawDebug(screen)
	}
}

// drawChromaAberration separates the R and B channels of the canvas by shifting them
// horizontally, creating a 3D anaglyph / chromatic aberration effect.
func (v *Visualizer) drawChromaAberration(screen, canvas *ebiten.Image) {
	shift := v.chromaShift
	// Glow intensity: proportional to the offset, max ~0.85
	alpha := float32(math.Min(shift/30.0, 1.0) * 0.85)

	// Center layer
	opBase := &ebiten.DrawImageOptions{}
	opBase.GeoM.Translate(v.shakeX, v.shakeY)
	screen.DrawImage(canvas, opBase)

	// Red glow to the right (additive: adds to what's already there)
	opR := &ebiten.DrawImageOptions{}
	opR.GeoM.Translate(shift+v.shakeX, v.shakeY)
	opR.ColorScale.Scale(1, 0, 0, alpha)
	opR.Blend = ebiten.BlendLighter
	screen.DrawImage(canvas, opR)

	// Blue glow to the left
	opB := &ebiten.DrawImageOptions{}
	opB.GeoM.Translate(-shift+v.shakeX, v.shakeY)
	opB.ColorScale.Scale(0, 0, 1, alpha)
	opB.Blend = ebiten.BlendLighter
	screen.DrawImage(canvas, opB)
}

func (v *Visualizer) drawRing(dst *ebiten.Image, cx, cy, radius float64) {
	n := NumBands
	barScale := 0.12 + v.ringPulse*0.06

	for i := 0; i < n; i++ {
		angle := 2*math.Pi*float64(i)/float64(n) - math.Pi/2
		mag := float64(v.bands[i])
		barLen := mag * float64(v.height) * barScale

		x0 := cx + math.Cos(angle)*radius
		y0 := cy + math.Sin(angle)*radius
		x1 := cx + math.Cos(angle)*(radius+barLen)
		y1 := cy + math.Sin(angle)*(radius+barLen)

		t := float64(i) / float64(n)
		bright := 0.7 + mag*0.3 + v.ringPulse*0.3
		if bright > 1.0 {
			bright = 1.0
		}
		c := hsvToRGB(v.hue+t*200, 0.9, bright)
		c.A = uint8(160 + v.ringPulse*80)

		vector.StrokeLine(dst,
			float32(x0), float32(y0),
			float32(x1), float32(y1),
			float32(1.2+mag*2.5+v.ringPulse*1.5), c, false)

		if v.peaks[i] > 0.02 {
			peakLen := float64(v.peaks[i]) * float64(v.height) * barScale
			px := float32(cx + math.Cos(angle)*(radius+peakLen))
			py := float32(cy + math.Sin(angle)*(radius+peakLen))
			pc := hsvToRGB(v.hue+t*200, 0.6, 1.0)
			pc.A = 200
			vector.DrawFilledCircle(dst, px, py, float32(1.5+v.ringPulse*2), pc, false)
		}
	}
}

func (v *Visualizer) drawShockwaves(dst *ebiten.Image, cx, cy float64) {
	steps := 120
	for _, sw := range v.shockwaves {
		a := uint8(sw.alpha * 255)
		c := hsvToRGB(sw.hue, 0.7, 1.0)
		c.A = a
		lineW := float32(sw.alpha * 3.0)
		if lineW < 0.5 {
			lineW = 0.5
		}
		for i := 0; i < steps; i++ {
			a0 := 2 * math.Pi * float64(i) / float64(steps)
			a1 := 2 * math.Pi * float64(i+1) / float64(steps)
			x0 := float32(cx + math.Cos(a0)*sw.radius)
			y0 := float32(cy + math.Sin(a0)*sw.radius)
			x1 := float32(cx + math.Cos(a1)*sw.radius)
			y1 := float32(cy + math.Sin(a1)*sw.radius)
			vector.StrokeLine(dst, x0, y0, x1, y1, lineW, c, true)
		}
	}
}

// drawGlitch draws horizontal slices of the canvas shifted sideways.
// The effect grows with buildup: from small shifts to large tears.
func (v *Visualizer) drawGlitch(screen, canvas *ebiten.Image, intensity float64) {
	n := int(intensity*18) + 2
	maxOffset := intensity * 100

	for i := 0; i < n; i++ {
		h := 8 + rand.Intn(int(intensity*60)+20)
		y := rand.Intn(v.height)
		if y+h > v.height {
			h = v.height - y
		}
		offsetX := (rand.Float64()*2 - 1) * maxOffset

		src := canvas.SubImage(image.Rect(0, y, v.width, y+h)).(*ebiten.Image)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(offsetX+v.shakeX, float64(y)+v.shakeY)
		op.ColorScale.Scale(1, 1, 1, float32(intensity*0.85))
		screen.DrawImage(src, op)
	}
}

// drawEnergyGraph draws the graph of a historical signal.
func (v *Visualizer) drawEnergyGraph(screen *ebiten.Image, label string, history []float64, writeIdx, x, y int, col color.RGBA) {
	const (
		graphW = 350.0
		graphH = 30.0
	)

	fx := float32(x)
	fy := float32(y)

	vector.DrawFilledRect(screen, fx, fy, graphW, graphH,
		color.RGBA{0, 0, 0, 140}, false)
	vector.StrokeRect(screen, fx, fy, graphW, graphH, 1,
		color.RGBA{80, 80, 80, 200}, false)
	ebitenutil.DebugPrintAt(screen, label, x+4, y+2)

	total := len(history)
	if writeIdx < 2 {
		return
	}

	var maxVal float64
	for _, val := range history {
		if val > maxVal {
			maxVal = val
		}
	}
	if maxVal < 0.001 {
		maxVal = 0.001
	}

	for i := 1; i < total; i++ {
		idx0 := (writeIdx - total + i - 1 + total*2) % total
		idx1 := (writeIdx - total + i + total*2) % total

		x0 := fx + float32(i-1)*graphW/float32(total)
		x1 := fx + float32(i)*graphW/float32(total)
		y0 := fy + graphH - float32(history[idx0]/maxVal)*graphH
		y1 := fy + graphH - float32(history[idx1]/maxVal)*graphH

		vector.StrokeLine(screen, x0, y0, x1, y1, 1, col, false)
	}
}

func (v *Visualizer) drawDebug(screen *ebiten.Image) {
	bpm := "BPM: --"
	if v.beat.BPM > 0 {
		bpm = fmt.Sprintf("BPM: %.1f", v.beat.BPM)
	}

	mode := "maj"
	if v.keyDet.IsMinor {
		mode = "min"
	}
	info := fmt.Sprintf(
		"F1: debug  F11: fullscreen  ESC: quit\n"+
			"FPS:        %.1f\n"+
			"%s  (speed: %.2f)\n"+
			"Key:        %s  %s %s  (conf: %.2f)\n"+
			"Chroma shift: %.1fpx\n"+
			"Ring pulse:   %.2f\n"+
			"AGC peak:     %.6f\n"+
			"Hihat rate:   %.1f/s\n"+
			"Buildup:      %.2f",
		ebiten.ActualFPS(),
		bpm, v.beat.SpeedFactor(),
		v.keyDet.Key, v.keyDet.Note, mode, v.keyDet.Confidence,
		v.chromaShift,
		v.ringPulse,
		v.agcPeak,
		v.beat.HihatRate,
		v.beat.BuildupIntensity,
	)

	ebitenutil.DebugPrintAt(screen, info, 10, 10)

	// Timeline graphs for kick, hi-hat and bassline (last 8 seconds)
	graphY := 220
	v.drawTimeline(screen, "KICK", v.beat.KickTimes(), 10, graphY,
		color.RGBA{255, 80, 60, 255})
	v.drawTimeline(screen, "HIHAT", v.beat.HihatTimes(), 10, graphY+50,
		color.RGBA{60, 200, 255, 255})
	v.drawEnergyGraph(screen, "BASS", v.bassHistory[:], v.historyIdx, 10, graphY+100,
		color.RGBA{255, 160, 40, 200})
	v.drawEnergyGraph(screen, "MIX", v.mixEnergyHistory[:], v.historyIdx, 10, graphY+150,
		color.RGBA{180, 255, 100, 200})
	v.drawEnergyGraph(screen, "HIHAT E", v.hihatEnergyHist[:], v.historyIdx, 10, graphY+200,
		color.RGBA{60, 200, 255, 160})
}

// drawTimeline draws a bar chart of detection timestamps over an 8-second window.
func (v *Visualizer) drawTimeline(screen *ebiten.Image, label string, times []time.Time, x, y int, col color.RGBA) {
	const (
		graphW  = 350.0
		graphH  = 30.0
		windowS = 8.0 // seconds of history
	)

	fx := float32(x)
	fy := float32(y)

	// Background
	vector.DrawFilledRect(screen, fx, fy, graphW, graphH,
		color.RGBA{0, 0, 0, 140}, false)
	// Border
	vector.StrokeRect(screen, fx, fy, graphW, graphH, 1,
		color.RGBA{80, 80, 80, 200}, false)

	// Label
	ebitenutil.DebugPrintAt(screen, label, x+4, y+2)

	now := time.Now()
	for _, t := range times {
		age := now.Sub(t).Seconds()
		if age > windowS {
			continue
		}
		// x: 0 = windowS seconds ago (left), graphW = now (right)
		barX := fx + float32((1.0-age/windowS)*graphW)
		// More recent bars are taller and more opaque
		freshness := float32(1.0 - age/windowS)
		barH := graphH * (0.3 + freshness*0.7)
		barY := fy + graphH - barH

		c := col
		c.A = uint8(80 + freshness*175)
		vector.DrawFilledRect(screen, barX-1, barY, 2, barH, c, false)
	}
}

func (v *Visualizer) Layout(outsideWidth, outsideHeight int) (int, int) {
	v.width = outsideWidth
	v.height = outsideHeight
	return outsideWidth, outsideHeight
}
