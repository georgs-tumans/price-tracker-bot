package main

import (
	"fmt"
	"log"
	"pricetrackerbot/botfixer"
	"strings"
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
			log.Fatalf("[main] Error deleting webhook: %v", err)
		}
		botFixer.InitializeBotLongPolling()
	} else {
		botFixer.InitializeBotWebhook()
	}
}
