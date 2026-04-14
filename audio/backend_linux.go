//go:build linux

package audio

import "github.com/gen2brain/malgo"

// On Linux, force PulseAudio backend (PipeWire-compatible) over raw ALSA.
// ALSA doesn't expose monitor/loopback sources needed for output capture.
var preferredBackends = []malgo.Backend{malgo.BackendPulseaudio}
