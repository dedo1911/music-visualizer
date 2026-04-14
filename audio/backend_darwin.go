//go:build darwin

package audio

import "github.com/gen2brain/malgo"

// On macOS, use Core Audio (the only native backend).
// Note: macOS does not natively support loopback capture.
// You need a virtual audio device like BlackHole (https://existential.audio/blackhole/)
// to route system audio to a capture device.
var preferredBackends = []malgo.Backend{malgo.BackendCoreaudio}
