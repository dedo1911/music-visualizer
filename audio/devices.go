package audio

import (
	"fmt"

	"github.com/gen2brain/malgo"
)

func ListDevices() error {
	ctx, err := malgo.InitContext(preferredBackends, malgo.ContextConfig{}, nil)
	if err != nil {
		return fmt.Errorf("init context: %w", err)
	}
	defer func() {
		ctx.Uninit()
		ctx.Free()
	}()

	devices, err := ctx.Devices(malgo.Capture)
	if err != nil {
		return fmt.Errorf("enumerate devices: %w", err)
	}

	fmt.Println("Available capture devices:")
	for i, d := range devices {
		fmt.Printf("  [%d] %s\n      id: %s\n", i, d.Name(), d.ID.String())
	}

	if len(devices) == 0 {
		fmt.Println("  (no devices found)")
	}

	fmt.Println()
	fmt.Println(LoopbackHelp())

	return nil
}
