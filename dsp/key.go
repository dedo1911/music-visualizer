package dsp

import (
	"math"
	"math/cmplx"
)

// Krumhansl-Schmuckler key profiles (from musicological literature)
var majorProfile = [12]float64{6.35, 2.23, 3.48, 2.33, 4.38, 4.09, 2.52, 5.19, 2.39, 3.66, 2.29, 2.88}
var minorProfile = [12]float64{6.33, 2.68, 3.52, 5.38, 2.60, 3.53, 2.54, 4.75, 3.98, 2.69, 3.34, 3.17}

// Camelot wheel: index = semitones from C (0=C, 1=C#, ..., 11=B)
var camelotMajor = [12]string{"8B", "3B", "10B", "5B", "12B", "7B", "2B", "9B", "4B", "11B", "6B", "1B"}
var camelotMinor = [12]string{"5A", "12A", "7A", "2A", "9A", "4A", "11A", "6A", "1A", "8A", "3A", "10A"}
var noteNames = [12]string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

// Camelot number (1-12) for each root note, used for color mapping.
// The Camelot number corresponds to the position on the circle of fifths.
var camelotNumMajor = [12]int{8, 3, 10, 5, 12, 7, 2, 9, 4, 11, 6, 1}
var camelotNumMinor = [12]int{5, 12, 7, 2, 9, 4, 11, 6, 1, 8, 3, 10}

// KeyDetector detects the musical key via chromatic analysis and K-S profiles.
// Accumulates the chromagram over a sliding window of ~5 seconds.
type KeyDetector struct {
	chroma [12]float64

	Key        string  // e.g. "8B"
	Note       string  // e.g. "C"
	IsMinor    bool
	Confidence float64 // Pearson correlation 0..1
	CamelotNum int     // 1-12, position on the circle of fifths
}

// BaseHue returns the hue (0-360°) associated with the current key.
// Camelot 1-12 → hue 0°-330°. Harmonically compatible keys
// (adjacent on the Camelot wheel) have similar colors.
// Minor → base hue, Major → hue + 15° (slight warm shift).
func (kd *KeyDetector) BaseHue() float64 {
	h := float64(kd.CamelotNum-1) * 30.0 // 1→0°, 2→30°, ..., 12→330°
	if !kd.IsMinor {
		h += 15.0
	}
	return math.Mod(h, 360)
}

func NewKeyDetector() *KeyDetector {
	return &KeyDetector{Key: "?", Note: "?"}
}

// Update refreshes the chromagram with current samples and recomputes the key.
func (kd *KeyDetector) Update(samples []float32) {
	n := len(samples)
	if n == 0 {
		return
	}

	// Hann window + FFT
	windowed := make([]float32, n)
	for i, s := range samples {
		w := float32(0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1))))
		windowed[i] = s * w
	}
	freqs := FFT(windowed)

	freqRes := float64(SampleRate) / float64(n)

	// Build the chromagram directly from the long buffer.
	// The ~6s buffer already integrates enough tonal context,
	// no need to accumulate with decay.
	for i := range kd.chroma {
		kd.chroma[i] = 0
	}
	for i := 1; i < n/2; i++ {
		freq := float64(i) * freqRes
		if freq < 27.5 || freq > 4200 {
			continue
		}
		mag := cmplx.Abs(freqs[i]) / float64(n)
		semitones := 12.0 * math.Log2(freq/440.0)
		pc := ((int(math.Round(semitones))%12 + 12 + 9) % 12)
		kd.chroma[pc] += mag
	}

	kd.detect()
}

func (kd *KeyDetector) detect() {
	// Normalize the chromagram
	var sum float64
	for _, v := range kd.chroma {
		sum += v
	}
	if sum < 1e-10 {
		return
	}
	var norm [12]float64
	for i, v := range kd.chroma {
		norm[i] = v / sum
	}

	// Pearson correlation with the 24 profiles (12 major + 12 minor)
	bestCorr := -math.MaxFloat64
	bestRoot := 0
	bestMinor := false

	for root := 0; root < 12; root++ {
		if c := pearson(norm, majorProfile, root); c > bestCorr {
			bestCorr, bestRoot, bestMinor = c, root, false
		}
		if c := pearson(norm, minorProfile, root); c > bestCorr {
			bestCorr, bestRoot, bestMinor = c, root, true
		}
	}

	kd.IsMinor = bestMinor
	kd.Note = noteNames[bestRoot]
	if bestMinor {
		kd.Key = camelotMinor[bestRoot]
		kd.CamelotNum = camelotNumMinor[bestRoot]
	} else {
		kd.Key = camelotMajor[bestRoot]
		kd.CamelotNum = camelotNumMajor[bestRoot]
	}
	// Confidence: map [-1,1] → [0,1]
	kd.Confidence = (bestCorr + 1) / 2
}

// pearson computes the Pearson correlation between the chromagram and the profile
// rotated by `shift` semitones.
func pearson(chroma [12]float64, profile [12]float64, shift int) float64 {
	var mC, mP float64
	for i := 0; i < 12; i++ {
		mC += chroma[(i+shift)%12]
		mP += profile[i]
	}
	mC /= 12
	mP /= 12

	var num, dC, dP float64
	for i := 0; i < 12; i++ {
		dc := chroma[(i+shift)%12] - mC
		dp := profile[i] - mP
		num += dc * dp
		dC += dc * dc
		dP += dp * dp
	}
	if dC < 1e-10 || dP < 1e-10 {
		return 0
	}
	return num / math.Sqrt(dC*dP)
}
