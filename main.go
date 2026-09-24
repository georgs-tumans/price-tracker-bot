package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"pricetrackerbot/botfixer"
)

// Release version, set at build time with -ldflags "-X main.version=..."; "dev" for local builds.
var version = "dev"

func main() {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("unhandled error: %v", r)
			log.Fatalf("[main] Unhandled panic: %v", err)

			return
		}
	}()

	// Stop cleanly on Ctrl+C and on `docker stop`
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("Starting bot service (version %s)", version)
	botfixer.NewBotFixer().Run(ctx)
}
