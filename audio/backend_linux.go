//go:build linux

package audio

import "github.com/gen2brain/malgo"

// On Linux, force PulseAudio backend (PipeWire-compatible) over raw ALSA.
// ALSA doesn't expose monitor/loopback sources needed for output capture.
var preferredBackends = []malgo.Backend{malgo.BackendPulseaudio}

// LoopbackHelp returns platform-specific instructions for setting up loopback capture.
func LoopbackHelp() string {
	return `To capture system audio output (loopback), run:
  pactl list sources short
  # find the line ending with .monitor, then:
  pactl set-default-source <name>.monitor

Or use -device <index> with a monitor from the list (-list).`
}
