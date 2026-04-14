//go:build windows

package audio

import "github.com/gen2brain/malgo"

// On Windows, use WASAPI which natively supports loopback capture.
var preferredBackends = []malgo.Backend{malgo.BackendWasapi}
