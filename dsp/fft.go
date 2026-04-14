package dsp

import (
	"math"
	"math/cmplx"
)

// FFT computes the Fast Fourier Transform of the input signal.
// Input length must be a power of 2.
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

// Spectrum computes the magnitude spectrum from samples.
// Returns normalized magnitudes for the first half (positive frequencies).
func Spectrum(samples []float32, numBands int) []float32 {
	n := len(samples)

	// Apply Hann window
	windowed := make([]float32, n)
	for i, s := range samples {
		w := float32(0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1))))
		windowed[i] = s * w
	}

	freqs := FFT(windowed)

	// Take magnitude of first half (positive frequencies)
	halfN := n / 2
	mags := make([]float32, halfN)
	for i := range mags {
		mags[i] = float32(cmplx.Abs(freqs[i])) / float32(n)
	}

	// Bin into numBands using logarithmic scale
	bands := make([]float32, numBands)
	minFreq := 20.0
	maxFreq := float64(SampleRate) / 2.0
	logMin := math.Log10(minFreq)
	logMax := math.Log10(maxFreq)

	for b := 0; b < numBands; b++ {
		freqLow := math.Pow(10, logMin+(logMax-logMin)*float64(b)/float64(numBands))
		freqHigh := math.Pow(10, logMin+(logMax-logMin)*float64(b+1)/float64(numBands))

		idxLow := int(freqLow / (float64(SampleRate) / float64(n)))
		idxHigh := int(freqHigh / (float64(SampleRate) / float64(n)))

		if idxLow >= halfN {
			break
		}
		if idxHigh > halfN {
			idxHigh = halfN
		}
		if idxHigh <= idxLow {
			idxHigh = idxLow + 1
		}

		var sum float32
		count := 0
		for i := idxLow; i < idxHigh && i < halfN; i++ {
			sum += mags[i]
			count++
		}
		if count > 0 {
			bands[b] = sum / float32(count)
		}
	}

	return bands
}

const SampleRate = 44100
