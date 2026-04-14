package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/dedo1911/music-visualizer/audio"
	"github.com/dedo1911/music-visualizer/visualizer"
	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	width := flag.Int("width", 1920, "window width")
	height := flag.Int("height", 1080, "window height")
	fullscreen := flag.Bool("fullscreen", false, "fullscreen mode")
	listDevices := flag.Bool("list", false, "list available audio capture devices")
	deviceIndex := flag.Int("device", -1, "capture device index from -list (-1 = default system source)")
	flag.Parse()

	if *listDevices {
		if err := audio.ListDevices(); err != nil {
			log.Fatalf("list devices: %v", err)
		}
		return
	}

	fmt.Println("Music Visualizer - avvio...")
	if *deviceIndex < 0 {
		fmt.Println()
		fmt.Println("Per catturare l'OUTPUT audio del PC (loopback), esegui:")
		fmt.Println("  pactl list sources short")
		fmt.Println("  # cerca la riga con .monitor, poi:")
		fmt.Println("  pactl set-default-source <nome>.monitor")
		fmt.Println()
		fmt.Println("Oppure usa -device <indice> con un monitor dalla lista (-list).")
		fmt.Println()
	} else {
		fmt.Printf("Usando device index: %d\n", *deviceIndex)
	}

	cap, err := audio.NewCapture(*deviceIndex)
	if err != nil {
		log.Fatalf("audio capture: %v\n\nEsegui con -list per vedere i dispositivi disponibili.", err)
	}

	if err := cap.Start(); err != nil {
		log.Fatalf("start capture: %v", err)
	}
	defer cap.Stop()

	vis := visualizer.New(cap, *width, *height)

	ebiten.SetWindowSize(*width, *height)
	ebiten.SetWindowTitle("Music Visualizer")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetFullscreen(*fullscreen)
	ebiten.SetTPS(60)
	ebiten.SetRunnableOnUnfocused(true)
	ebiten.SetScreenClearedEveryFrame(true)

	if err := ebiten.RunGame(vis); err != nil {
		log.Fatalf("run: %v", err)
	}
}
