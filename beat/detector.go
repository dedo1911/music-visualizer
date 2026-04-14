package beat

import (
	"math"
	"time"
)

const (
	// Kick drum: ~27-60Hz → bande 5-20
	kickStartBand     = 5
	kickEndBand       = 20
	kickFluxThreshold = 0.30
	kickSmoothAlpha   = 0.35 // bilanciamento tra reattività e selezione transienti
	kickAgcDecay      = 0.9995

	// Hi-hat / cymbal: ~5kHz-12kHz → bande 101-120
	hihatStartBand     = 101
	hihatEndBand       = 120
	hihatFluxThreshold = 0.30
	hihatSmoothAlpha   = 0.35
	hihatAgcDecay      = 0.9992

	minBeatGap   = 300 * time.Millisecond
	hihatMinGap  = 60 * time.Millisecond // hi-hat può essere molto rapido
	maxIntervals = 8
)

// Detector rileva kick, hi-hat e buildup tramite spectral flux onset detection.
type Detector struct {
	lastBeat     time.Time
	lastHihat    time.Time
	intervals    []float64
	kickAgcPeak  float32
	kickSmooth   float32
	hihatAgcPeak float32
	hihatSmooth  float32
	hihatTimes   []time.Time // timestamp degli ultimi hi-hat per calcolare il rate
	kickTimes    []time.Time // timestamp degli ultimi kick per il grafico debug

	BPM              float64
	IsKick           bool
	KickStrength     float64 // 0..1
	BassEnergy       float64
	IsHihat          bool
	HihatStrength    float64 // 0..1
	HihatEnergy      float64
	HihatRate        float64 // hits/secondo (media su 2s)
	BuildupIntensity float64 // 0..1, alto durante buildup (hi-hat roll)
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

	// Pulizia kick timestamps (tieni ultimi 8 secondi)
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

	// Calcola hi-hat rate: quanti hit negli ultimi 2 secondi
	const rateWindow = 2 * time.Second
	const historyWindow = 8 * time.Second
	rateCutoff := now.Add(-rateWindow)
	historyCutoff := now.Add(-historyWindow)

	// Conta hits nel rate window
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
	// Normale: 2-4 hits/s (8ths a 120-140 BPM)
	// Buildup: 8-16+ hits/s (32nds, 64ths, roll)
	// Le soglie scalano col BPM: a BPM più alto il rate "normale" è più alto,
	// quindi servono più hits/s per essere considerato buildup.
	// Base: 8th notes = BPM/60*2 hits/s (hi-hat standard su ottavi)
	// Buildup: 32nd notes = BPM/60*8 hits/s (roll a trentaduesimi)
	eighths := d.BPM / 60.0 * 2.0 // rate di ottavi al BPM corrente
	if eighths < 3.0 {
		eighths = 3.0 // fallback se BPM non ancora rilevato
	}
	rateNormal := eighths * 1.8  // margine più alto sopra il groove normale
	rateBuildup := eighths * 4.5 // richiede un roll molto denso

	// Buildup basato solo sul rate percussivo: solo i transienti
	// rapidi contano, l'energia sostenuta (voci, synth) no.
	bi := (d.HihatRate - rateNormal) / (rateBuildup - rateNormal)
	if bi < 0 {
		bi = 0
	}
	if bi > 1 {
		bi = 1
	}
	// Smoothing con decay adattivo:
	// - Sale veloce (cattura subito il roll)
	// - Scende lento durante i piccoli gap nel roll
	// - Scende MOLTO veloce se l'energia crolla (break/silenzio prima del drop)
	if bi > d.BuildupIntensity {
		d.BuildupIntensity = d.BuildupIntensity*0.85 + bi*0.15
	} else if d.HihatEnergy < 0.1 && d.BassEnergy < 0.1 {
		// Silenzio/break: kill rapido
		d.BuildupIntensity *= 0.92
	} else {
		// Gap normale nel roll: decay lento
		d.BuildupIntensity = d.BuildupIntensity*0.998 + bi*0.002
	}
}

// detectFlux esegue spectral flux onset detection su un range di bande.
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

// KickTimes restituisce i timestamp dei kick recenti.
func (d *Detector) KickTimes() []time.Time { return d.kickTimes }

// HihatTimes restituisce i timestamp degli hi-hat recenti.
func (d *Detector) HihatTimes() []time.Time { return d.hihatTimes }

// SpeedFactor mappa il range DJ reale (124-155 BPM) su 0.6..2.2.
// Sotto 124 BPM → 0.6 (minimo), sopra 155 BPM → 2.2 (massimo).
// La differenza visiva tra 124 e 155 BPM è quindi 3.7x invece di 1.25x.
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
