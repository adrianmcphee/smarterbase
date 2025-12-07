// SmarterBase - PostgreSQL compatible file store
//
// Iterate on your data model without migrations.
// Your data is JSON files you can see and edit.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/adrianmcphee/smarterbase/internal/export"
	"github.com/adrianmcphee/smarterbase/internal/protocol"
	"github.com/adrianmcphee/smarterbase/internal/storage"
)

func main() {
	// Check for subcommands first
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "export":
			runExport(os.Args[2:])
			return
		case "help", "--help", "-h":
			printHelp()
			return
		}
	}

	// Default: run server
	runServer()
}

func printHelp() {
	fmt.Println(`SmarterBase - PostgreSQL compatible file store

Usage:
  smarterbase [flags]              Start the server
  smarterbase export [flags]       Export schema and data to PostgreSQL

Server flags:
  --port int     Port to listen on (default 5433)
  --data string  Data directory (default "./data")

Export flags:
  --data string  Data directory (default "./data")
  --ddl-only     Export only schema (no data)
  --data-only    Export only data (no schema)`)
}

func runServer() {
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

func runExport(args []string) {
	exportFlags := flag.NewFlagSet("export", flag.ExitOnError)
	dataDir := exportFlags.String("data", "./data", "Data directory")
	ddlOnly := exportFlags.Bool("ddl-only", false, "Export only schema (no data)")
	dataOnly := exportFlags.Bool("data-only", false, "Export only data (no schema)")
	exportFlags.Parse(args)

	// Open the store
	store, err := storage.NewStore(*dataDir)
	if err != nil {
		log.Fatalf("Failed to open data directory: %v", err)
	}

	// Generate export
	var output string
	switch {
	case *ddlOnly:
		output = export.ExportDDL(store)
	case *dataOnly:
		output = export.ExportData(store)
	default:
		output = export.Export(store)
	}

	fmt.Print(output)
}
