package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"brave-scraper/lyrics"

	"github.com/hugolgst/rich-go/client"
)

func main() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\nTerminating program and closing floating window...")
		lyrics.Cleanup()
		os.Exit(0)
	}()

	discordClientID := os.Getenv("DISCORD_CLIENT_ID")
	if discordClientID == "" {
		log.Println("Warning: DISCORD_CLIENT_ID environment variable is not set.")
	}

	err := client.Login(discordClientID)
	if err != nil {
		log.Printf("Error connecting to Discord: %v\n", err)
	} else {
		fmt.Println("Connected to Discord Rich Presence!")
		defer client.Logout()
	}

	fmt.Println("Starting tab and music time monitoring...")

	go startMonitoring()

	lyrics.ShowUI()
}
