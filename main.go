package main

import (
	"fmt"
	"log"
	"strings"

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

	log.Println("Starting bot service")
	botFixer := botfixer.NewBotFixer()
	config := botFixer.Config

	if strings.ToLower(strings.TrimSpace(config.Environment)) == "local" {
		if err := botFixer.DeleteWebhook(); err != nil {
			log.Printf("[main] Error deleting webhook: %v", err)
		}
		botFixer.InitializeBotLongPolling()
	} else {
		botFixer.InitializeBotWebhook()
	}
}
