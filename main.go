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

	log.Println("Starting bot service")
	botfixer.NewBotFixer().Run(ctx)
}
