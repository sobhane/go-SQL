package main

import (
	"fmt"
	"os"

	"github.com/sobhane/golang-database/engine"
	"github.com/sobhane/golang-database/repl"
	"github.com/sobhane/golang-database/storage"
)

const defaultDBPath = "mydb.db"

func main() {
	dbPath := defaultDBPath
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	// Open (or create) the database file
	pager, err := storage.OpenPager(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %s\n", err)
		os.Exit(1)
	}
	defer pager.Close()

	// Load (or create) the table catalog
	catalog, err := engine.NewCatalog(pager)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load catalog: %s\n", err)
		os.Exit(1)
	}

	// Create the query executor
	executor := engine.NewExecutor(pager, catalog)

	// Start the interactive REPL
	r := repl.New(executor)

	// Catch the "quit" panic from the REPL
	defer func() {
		if r := recover(); r != nil {
			if r == "quit" {
				os.Exit(0)
			}
			panic(r) // re-panic for real errors
		}
	}()

	r.Start(os.Stdin, os.Stdout)
}
