package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	client := NewClient(cfg)
	if err := client.Connect(); err != nil {
		log.Fatalf("connect error: %v", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf("close error: %v", err)
		}
	}()

	// Send an example SMS asynchronously (adjust as needed)
	go func() {
		// give session a moment to bind
		time.Sleep(2 * time.Second)
		src := cfg.SourceAddr
		if src == "" {
			src = "MelroseLabs"
		}
		dst := cfg.DestAddr
		if dst == "" {
			dst = "447712345678"
		}
		sub := NewSubmitSM(src, dst, "Hello World")
		if err := client.SendSMS(sub); err != nil {
			log.Printf("failed to submit sms: %v", err)
		} else {
			log.Println("submit sent")
		}
	}()

	// Wait for interrupt to exit gracefully
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("shutting down")
}
