// SmarterBase - PostgreSQL compatible file store
//
// Iterate on your data model without migrations.
// Your data is JSON files you can see and edit.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/adrianmcphee/smarterbase/v2/internal/protocol"
)

func main() {
	var (
		port    = flag.Int("port", 5433, "Port to listen on")
		dataDir = flag.String("data", "./data", "Data directory")
	)
	flag.Parse()

	log.SetFlags(log.Ltime | log.Lshortfile)

	// Ensure data directory exists
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	log.Printf("SmarterBase starting...")
	log.Printf("Data directory: %s", *dataDir)

	server, err := protocol.NewServer(*port, *dataDir)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := server.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
