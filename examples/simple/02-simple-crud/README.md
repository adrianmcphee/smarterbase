# Simple CRUD Example

Demonstrates the four basic operations: Create, Read, Update, Delete.

## What it does

- Creates a task with auto-generated ID
- Reads the task back by ID
- Updates the task (marks it as done)
- Counts total tasks
- Deletes the task
- Verifies deletion

## Running

```bash
# From this directory
go run main.go
```

## Expected Output

```
=== CREATE ===
Created task: Learn SmarterBase (ID: task-abc123)

=== READ ===
Found task: Learn SmarterBase
Description: Go through the examples
Done: false

=== UPDATE ===
Task marked as done
Updated task: Learn SmarterBase (Done: true)

=== COUNT ===
Total tasks: 1

=== DELETE ===
Task deleted
Confirmed: task no longer exists
Total tasks after delete: 0
```

## Key Concepts

### Immutable Create

```go
task := &Task{Title: "Learn SmarterBase"}
created, err := tasks.Create(ctx, task)
// task.ID is still empty (unchanged)
// created.ID is populated
```

The Create method returns a new object with the ID populated, leaving your input unchanged. This prevents surprising mutations.

### Update Requires ID

```go
found.Done = true
err := tasks.Update(ctx, found)
```

The Update method requires the ID field to be set. Get the object first, modify it, then update.

### Count and Iteration

```go
count, err := tasks.Count(ctx)  // Get total count

// Or iterate without loading all into memory
err := tasks.Each(ctx, func(task *Task) error {
    fmt.Println(task.Title)
    return nil
})
```

Use `Count()` for totals, `Each()` for streaming iteration, `All()` to load everything.

## Next Steps

- See [03-with-indexing](../03-with-indexing) for queries and indexes
