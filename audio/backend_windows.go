//go:build windows

package audio

import "github.com/gen2brain/malgo"

// On Windows, use WASAPI which natively supports loopback capture.
var preferredBackends = []malgo.Backend{malgo.BackendWasapi}

// LoopbackHelp returns platform-specific instructions for setting up loopback capture.
func LoopbackHelp() string {
	return `On Windows, WASAPI supports loopback capture natively.
The default capture device should work automatically.
If not, use -list to see available devices and -device <index> to select one.`
}
