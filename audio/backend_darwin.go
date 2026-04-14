//go:build darwin

package audio

import "github.com/gen2brain/malgo"

// On macOS, use Core Audio (the only native backend).
// macOS does not natively support loopback capture.
// A virtual audio device like BlackHole is required.
var preferredBackends = []malgo.Backend{malgo.BackendCoreaudio}

// LoopbackHelp returns platform-specific instructions for setting up loopback capture.
func LoopbackHelp() string {
	return `macOS does not support loopback capture natively.
Install BlackHole (https://existential.audio/blackhole/) then:
  1. Open Audio MIDI Setup
  2. Create a Multi-Output Device with your speakers + BlackHole
  3. Set the Multi-Output Device as system output
  4. Run this visualizer with -device pointing to BlackHole

Or use -device <index> from the list (-list).`
}
