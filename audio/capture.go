package audio

import (
	"fmt"
	"math"
	"sync"

	"github.com/gen2brain/malgo"
)

const (
	SampleRate  = 44100
	Channels    = 2
	FFTSize     = 2048
	KeyFFTSize  = 262144 // buffer lungo per key detection (~5.9s, risoluzione ~0.17Hz)
)

// pulseBackends forces PulseAudio (PipeWire-compatible) over raw ALSA.
var pulseBackends = []malgo.Backend{malgo.BackendPulseaudio}

type Capture struct {
	mu         sync.RWMutex
	samples    []float32 // FFTSize, per spectrum/beat
	keySamples []float32 // KeyFFTSize, per key detection
	device     *malgo.Device
	ctx        *malgo.AllocatedContext
}

// NewCapture opens an audio capture device using PulseAudio backend.
// deviceIndex: -1 = default source (set via pactl set-default-source)
//
//	>=0 = index from the list returned by ListDevices()
func NewCapture(deviceIndex int) (*Capture, error) {
	ctx, err := malgo.InitContext(pulseBackends, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("init pulseaudio context: %w", err)
	}

	c := &Capture{
		samples:    make([]float32, FFTSize),
		keySamples: make([]float32, KeyFFTSize),
		ctx:        ctx,
	}

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatF32
	deviceConfig.Capture.Channels = Channels
	deviceConfig.SampleRate = SampleRate
	deviceConfig.PeriodSizeInFrames = 256 // ~5.8ms per callback, riduce la latenza

	if deviceIndex >= 0 {
		devices, err := ctx.Devices(malgo.Capture)
		if err != nil {
			ctx.Uninit()
			ctx.Free()
			return nil, fmt.Errorf("enumerate devices: %w", err)
		}
		if deviceIndex >= len(devices) {
			ctx.Uninit()
			ctx.Free()
			return nil, fmt.Errorf("device index %d out of range (found %d devices)", deviceIndex, len(devices))
		}
		id := devices[deviceIndex].ID
		deviceConfig.Capture.DeviceID = id.Pointer()
	}

	callbacks := malgo.DeviceCallbacks{
		Data: c.onData,
	}

	device, err := malgo.InitDevice(ctx.Context, deviceConfig, callbacks)
	if err != nil {
		ctx.Uninit()
		ctx.Free()
		return nil, fmt.Errorf("init device: %w", err)
	}

	c.device = device
	return c, nil
}

func (c *Capture) onData(pOutputSample, pInputSample []byte, frameCount uint32) {
	floats := bytesToFloat32(pInputSample)

	// Mix to mono
	mono := make([]float32, int(frameCount))
	for i := range mono {
		if i*int(Channels)+1 < len(floats) {
			mono[i] = (floats[i*int(Channels)] + floats[i*int(Channels)+1]) * 0.5
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	newLen := len(mono)

	// Buffer corto per spectrum/beat
	if newLen >= FFTSize {
		c.samples = mono[newLen-FFTSize:]
	} else {
		c.samples = append(c.samples[newLen:], mono...)
	}

	// Buffer lungo per key detection
	if newLen >= KeyFFTSize {
		c.keySamples = mono[newLen-KeyFFTSize:]
	} else {
		c.keySamples = append(c.keySamples[newLen:], mono...)
	}
}

func (c *Capture) GetSamples() []float32 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]float32, len(c.samples))
	copy(result, c.samples)
	return result
}

func (c *Capture) GetKeySamples() []float32 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]float32, len(c.keySamples))
	copy(result, c.keySamples)
	return result
}

func (c *Capture) Start() error {
	return c.device.Start()
}

func (c *Capture) Stop() {
	c.device.Stop()
	c.device.Uninit()
	c.ctx.Uninit()
	c.ctx.Free()
}

func bytesToFloat32(b []byte) []float32 {
	if len(b) == 0 {
		return nil
	}
	f := make([]float32, len(b)/4)
	for i := range f {
		bits := uint32(b[i*4]) | uint32(b[i*4+1])<<8 | uint32(b[i*4+2])<<16 | uint32(b[i*4+3])<<24
		f[i] = math.Float32frombits(bits)
	}
	return f
}
