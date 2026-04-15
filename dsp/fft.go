package dsp

import (
	"math"
	"math/cmplx"
	"sync"
)

// SampleRate is the audio sample rate used throughout the application.
// Also defined in audio.SampleRate — kept separate to avoid import cycles.
const SampleRate = 44100

// Recursive Cooley-Tukey FFT. Input length must be a power of 2.
func FFT(x []float32) []complex128 {
	n := len(x)
	c := make([]complex128, n)
	for i, v := range x {
		c[i] = complex(float64(v), 0)
	}
	fft(c)
	return c
}

func fft(a []complex128) {
	n := len(a)
	if n <= 1 {
		return
	}

	even := make([]complex128, n/2)
	odd := make([]complex128, n/2)
	for i := 0; i < n/2; i++ {
		even[i] = a[i*2]
		odd[i] = a[i*2+1]
	}

	fft(even)
	fft(odd)

	for k := 0; k < n/2; k++ {
		t := cmplx.Exp(complex(0, -2*math.Pi*float64(k)/float64(n))) * odd[k]
		a[k] = even[k] + t
		a[k+n/2] = even[k] - t
	}
}

// HannWindow applies a Hann window to the samples in-place into dst.
// dst must be len(samples). Reuse dst across calls to avoid allocations.
func HannWindow(samples []float32, dst []float32, window []float64) {
	for i, s := range samples {
		dst[i] = s * float32(window[i])
	}
}

// PrecomputeHannWindow returns precomputed Hann window coefficients for size n.
func PrecomputeHannWindow(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1)))
	}
	return w
}

// bandBoundary holds precomputed FFT bin indices for a spectrum band.
type bandBoundary struct {
	lo, hi int
}

// spectrumCache stores precomputed data for a given FFT size and band count.
type spectrumCache struct {
	window     []float64
	windowed   []float32
	mags       []float32
	bands      []float32
	boundaries []bandBoundary
	fftSize    int
	numBands   int
}

var (
	specCache     *spectrumCache
	specCacheMu   sync.Mutex
)

func getSpectrumCache(n, numBands int) *spectrumCache {
	specCacheMu.Lock()
	defer specCacheMu.Unlock()

	if specCache != nil && specCache.fftSize == n && specCache.numBands == numBands {
		return specCache
	}

	halfN := n / 2
	minFreq := 20.0
	maxFreq := float64(SampleRate) / 2.0
	logMin := math.Log10(minFreq)
	logMax := math.Log10(maxFreq)
	freqRes := float64(SampleRate) / float64(n)

	boundaries := make([]bandBoundary, numBands)
	for b := 0; b < numBands; b++ {
		freqLow := math.Pow(10, logMin+(logMax-logMin)*float64(b)/float64(numBands))
		freqHigh := math.Pow(10, logMin+(logMax-logMin)*float64(b+1)/float64(numBands))

		lo := int(freqLow / freqRes)
		hi := int(freqHigh / freqRes)

		if lo >= halfN {
			lo = halfN - 1
		}
		if hi > halfN {
			hi = halfN
		}
		if hi <= lo {
			hi = lo + 1
		}
		boundaries[b] = bandBoundary{lo, hi}
	}

	specCache = &spectrumCache{
		window:     PrecomputeHannWindow(n),
		windowed:   make([]float32, n),
		mags:       make([]float32, halfN),
		bands:      make([]float32, numBands),
		boundaries: boundaries,
		fftSize:    n,
		numBands:   numBands,
	}
	return specCache
}

// Spectrum computes the magnitude spectrum from samples using precomputed
// window coefficients and band boundaries to avoid per-frame allocations.
func Spectrum(samples []float32, numBands int) []float32 {
	n := len(samples)
	sc := getSpectrumCache(n, numBands)

	HannWindow(samples, sc.windowed, sc.window)
	freqs := FFT(sc.windowed)

	halfN := n / 2
	for i := 0; i < halfN; i++ {
		sc.mags[i] = float32(cmplx.Abs(freqs[i])) / float32(n)
	}

	for b := 0; b < numBands; b++ {
		bb := sc.boundaries[b]
		var sum float32
		count := 0
		for i := bb.lo; i < bb.hi && i < halfN; i++ {
			sum += sc.mags[i]
			count++
		}
		if count > 0 {
			sc.bands[b] = sum / float32(count)
		} else {
			sc.bands[b] = 0
		}
	}

	// Return a copy since caller may store the reference
	result := make([]float32, numBands)
	copy(result, sc.bands)
	return result
}
