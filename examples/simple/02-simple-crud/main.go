// Simple CRUD demonstrates Create, Read, Update, Delete operations
// with the Simple API.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/adrianmcphee/smarterbase/v2/simple"
)

type Task struct {
	ID          string `json:"id" sb:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Done        bool   `json:"done"`
}

func main() {
	ctx := context.Background()

	// Setup
	db := simple.MustConnect()
	defer db.Close()

	tasks := simple.NewCollection[Task](db)

	// CREATE
	fmt.Println("=== CREATE ===")
	task := &Task{
		Title:       "Learn SmarterBase",
		Description: "Go through the examples",
		Done:        false,
	}

	created, err := tasks.Create(ctx, task)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created task: %s (ID: %s)\n", created.Title, created.ID)

	// READ
	fmt.Println("\n=== READ ===")
	found, err := tasks.Get(ctx, created.ID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Found task: %s\n", found.Title)
	fmt.Printf("Description: %s\n", found.Description)
	fmt.Printf("Done: %v\n", found.Done)

	// UPDATE
	fmt.Println("\n=== UPDATE ===")
	found.Done = true
	found.Description = "Completed all examples!"
	if err := tasks.Update(ctx, found); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Task marked as done")

	// Verify update
	updated, err := tasks.Get(ctx, created.ID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Updated task: %s (Done: %v)\n", updated.Title, updated.Done)

	// COUNT
	fmt.Println("\n=== COUNT ===")
	count, err := tasks.Count(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Total tasks: %d\n", count)

	// DELETE
	fmt.Println("\n=== DELETE ===")
	if err := tasks.Delete(ctx, created.ID); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Task deleted")

	// Verify deletion
	_, err = tasks.Get(ctx, created.ID)
	if err != nil {
		fmt.Printf("Confirmed: task no longer exists\n")
	}

	// Final count
	count, err = tasks.Count(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Total tasks after delete: %d\n", count)
}
