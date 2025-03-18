package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	server "github.com/dimeko/milcon/internal"
)

func main() {
	exitChannel := make(chan os.Signal, 1)
	signal.Notify(exitChannel, syscall.SIGINT, syscall.SIGTERM)
	srv := server.NewServer("6767")
	if err := srv.Start(exitChannel); err != nil {
		log.Fatalf("failed to start WS server: %w", err)
	}
	defer srv.Stop()

	log.Println("Starting server")
	<-exitChannel
}
