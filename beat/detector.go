package beat

import (
	"math"
	"time"
)

const (
	// Kick drum: ~27-60Hz → bands 5-20
	kickStartBand     = 5
	kickEndBand       = 20
	kickFluxThreshold = 0.30
	kickSmoothAlpha   = 0.35 // balance between reactivity and transient selection
	kickAgcDecay      = 0.9995

	// Hi-hat / cymbal: ~5kHz-12kHz → bands 101-120
	hihatStartBand     = 101
	hihatEndBand       = 120
	hihatFluxThreshold = 0.30
	hihatSmoothAlpha   = 0.35
	hihatAgcDecay      = 0.9992

	minBeatGap   = 300 * time.Millisecond
	hihatMinGap  = 60 * time.Millisecond // hi-hat can be very fast
	maxIntervals = 8
)

// Detector detects kick, hi-hat and buildup via spectral flux onset detection.
type Detector struct {
	lastBeat     time.Time
	lastHihat    time.Time
	intervals    []float64
	kickAgcPeak  float32
	kickSmooth   float32
	hihatAgcPeak float32
	hihatSmooth  float32
	hihatTimes   []time.Time // timestamps of recent hi-hats to compute rate
	kickTimes    []time.Time // timestamps of recent kicks for debug graph

	BPM              float64
	IsKick           bool
	KickStrength     float64 // 0..1
	BassEnergy       float64
	IsHihat          bool
	HihatStrength    float64 // 0..1
	HihatEnergy      float64
	HihatRate        float64 // hits/second (average over 2s)
	BuildupIntensity float64 // 0..1, high during buildup (hi-hat roll)
}

func (d *Detector) Update(bands []float32) {
	d.IsKick = false
	d.IsHihat = false

	now := time.Now()

	d.IsKick, d.KickStrength, d.BassEnergy = d.detectFlux(
		bands, kickStartBand, kickEndBand,
		kickSmoothAlpha, kickFluxThreshold,
		&d.kickAgcPeak, kickAgcDecay, &d.kickSmooth,
	)
	if d.IsKick {
		if now.Sub(d.lastBeat) < minBeatGap {
			d.IsKick = false
		} else {
			if !d.lastBeat.IsZero() {
				interval := now.Sub(d.lastBeat).Seconds()
				if interval < 2.0 {
					d.intervals = append(d.intervals, interval)
					if len(d.intervals) > maxIntervals {
						d.intervals = d.intervals[1:]
					}
					if len(d.intervals) >= 2 {
						var sum float64
						for _, iv := range d.intervals {
							sum += iv
						}
						d.BPM = 60.0 / (sum / float64(len(d.intervals)))
					}
				}
			}
			d.lastBeat = now
			d.kickTimes = append(d.kickTimes, now)
		}
	}

	// Clean up kick timestamps (keep last 8 seconds)
	kickCutoff := now.Add(-8 * time.Second)
	firstValidKick := 0
	for firstValidKick < len(d.kickTimes) && d.kickTimes[firstValidKick].Before(kickCutoff) {
		firstValidKick++
	}
	d.kickTimes = d.kickTimes[firstValidKick:]

	d.IsHihat, d.HihatStrength, d.HihatEnergy = d.detectFlux(
		bands, hihatStartBand, hihatEndBand,
		hihatSmoothAlpha, hihatFluxThreshold,
		&d.hihatAgcPeak, hihatAgcDecay, &d.hihatSmooth,
	)
	if d.IsHihat && now.Sub(d.lastHihat) < hihatMinGap {
		d.IsHihat = false
	} else if d.IsHihat {
		d.lastHihat = now
		d.hihatTimes = append(d.hihatTimes, now)
	}

	// Compute hi-hat rate: how many hits in the last 2 seconds
	const rateWindow = 2 * time.Second
	const historyWindow = 8 * time.Second
	rateCutoff := now.Add(-rateWindow)
	historyCutoff := now.Add(-historyWindow)

	// Count hits in the rate window
	rateCount := 0
	firstValid := 0
	for i, t := range d.hihatTimes {
		if t.Before(historyCutoff) {
			firstValid = i + 1
		}
		if !t.Before(rateCutoff) {
			rateCount++
		}
	}
	d.hihatTimes = d.hihatTimes[firstValid:]
	d.HihatRate = float64(rateCount) / rateWindow.Seconds()

	// Buildup detection: hi-hat rate mapping
	// Normal: 2-4 hits/s (8ths at 120-140 BPM)
	// Buildup: 8-16+ hits/s (32nds, 64ths, roll)
	// Thresholds scale with BPM: at higher BPM the "normal" rate is higher,
	// so more hits/s are needed to be considered buildup.
	// Base: 8th notes = BPM/60*2 hits/s (standard hi-hat on eighth notes)
	// Buildup: 32nd notes = BPM/60*8 hits/s (thirty-second note roll)
	eighths := d.BPM / 60.0 * 2.0 // eighth note rate at current BPM
	if eighths < 3.0 {
		eighths = 3.0 // fallback if BPM not yet detected
	}
	rateNormal := eighths * 1.8  // higher margin above normal groove
	rateBuildup := eighths * 4.5 // requires a very dense roll

	// Buildup based only on percussive rate: only fast transients
	// count, sustained energy (vocals, synth) does not.
	bi := (d.HihatRate - rateNormal) / (rateBuildup - rateNormal)
	if bi < 0 {
		bi = 0
	}
	if bi > 1 {
		bi = 1
	}
	// Smoothing with adaptive decay:
	// - Rises fast (catches the roll immediately)
	// - Falls slowly during small gaps in the roll
	// - Falls VERY fast if energy drops (break/silence before the drop)
	if bi > d.BuildupIntensity {
		d.BuildupIntensity = d.BuildupIntensity*0.85 + bi*0.15
	} else if d.HihatEnergy < 0.1 && d.BassEnergy < 0.1 {
		// Silence/break: fast kill
		d.BuildupIntensity *= 0.92
	} else {
		// Normal gap in the roll: slow decay
		d.BuildupIntensity = d.BuildupIntensity*0.998 + bi*0.002
	}
}

// detectFlux performs spectral flux onset detection on a range of bands.
func (d *Detector) detectFlux(
	bands []float32, start, end int,
	smoothAlpha, threshold float32,
	agcPeak *float32, agcDecay float32, smoothed *float32,
) (isOnset bool, strength, energy float64) {
	if end > len(bands) {
		end = len(bands)
	}
	if start >= end {
		return
	}
	var raw float32
	n := end - start
	for i := start; i < end; i++ {
		raw += bands[i]
	}
	raw /= float32(n)

	if raw > *agcPeak {
		*agcPeak = raw
	} else {
		*agcPeak *= agcDecay
	}
	if *agcPeak < 1e-9 {
		*agcPeak = 1e-9
	}
	norm := raw / *agcPeak

	prev := *smoothed
	*smoothed = prev*(1-smoothAlpha) + norm*smoothAlpha
	energy = float64(*smoothed)

	flux := norm - prev
	if flux > threshold {
		isOnset = true
		strength = math.Min(float64(flux/threshold), 1.0)
	}
	return
}

// KickTimes returns the timestamps of recent kicks.
func (d *Detector) KickTimes() []time.Time { return d.kickTimes }

// HihatTimes returns the timestamps of recent hi-hats.
func (d *Detector) HihatTimes() []time.Time { return d.hihatTimes }

// SpeedFactor maps the real DJ range (124-155 BPM) to 0.6..2.2.
// Below 124 BPM → 0.6 (minimum), above 155 BPM → 2.2 (maximum).
// The visual difference between 124 and 155 BPM is thus 3.7x instead of 1.25x.
func (d *Detector) SpeedFactor() float64 {
	if d.BPM <= 0 {
		return 1.0
	}
	const (
		bpmLow  = 124.0
		bpmHigh = 155.0
		outLow  = 0.6
		outHigh = 2.2
	)
	t := (d.BPM - bpmLow) / (bpmHigh - bpmLow)
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return outLow + t*(outHigh-outLow)
}
