# Quickstart: Coffee Tracker with Auto-Indexing

**The wow factor: Indexes from struct tags.**

Put `sb:"index"` on a field. Query it instantly. That's it.

## Running

```bash
# Make sure Redis/Valkey is running
docker run -d -p 6379:6379 redis  # or valkey

# Run the tracker multiple times
go run main.go
# ☕ Coffee #1 logged!
#    Total Espressos: 1
#    Total Doubles: 1
# 💡 The indexes were created automatically from struct tags.

go run main.go
# ☕ Coffee #2 logged!
#    Total Espressos: 2
#    Total Doubles: 2

go run main.go
# ☕ Coffee #3 logged!
#    Total Espressos: 3
#    Total Doubles: 3
```

## The Magic: Automatic Indexing

Look at the struct:

```go
type Coffee struct {
    ID      string    `json:"id" sb:"id"`
    Type    string    `json:"type" sb:"index"`     // ← Just add this tag
    Size    string    `json:"size" sb:"index"`     // ← And this tag
    DrankAt time.Time `json:"drank_at"`
}
```

**Now you can query:**

```go
espressos, _ := coffees.Find(ctx, "type", "Espresso")
doubles, _ := coffees.Find(ctx, "size", "Double")
```

**That's the wow factor.** No index registration code. No migrations. Just tags.

## What's Happening Behind the Scenes?

1. **Struct Tag Parsing**: `NewCollection[Coffee](db)` scans your struct at startup
2. **Redis Index Registration**: Creates `coffees-by-type` and `coffees-by-size` indexes automatically
3. **Auto Index Updates**: Every `Create()` and `Update()` maintains the indexes
4. **Type-Safe Queries**: `Find(ctx, "type", "Espresso")` returns `[]*Coffee` with compile-time safety

## Compare to Traditional Approach

**With SQL:**
```sql
CREATE TABLE coffees (
    id VARCHAR PRIMARY KEY,
    type VARCHAR,
    size VARCHAR,
    drank_at TIMESTAMP
);
CREATE INDEX idx_coffee_type ON coffees(type);
CREATE INDEX idx_coffee_size ON coffees(size);
```

**With SmarterBase:**
```go
type Coffee struct {
    Type string `sb:"index"`
    Size string `sb:"index"`
}
```

3 lines of DDL → 2 struct tags. ✨

## Where's the Data?

**Files** in `./data/coffees/`:
```bash
ls data/coffees/
# 0199e3da-fcf9-762e-821e-57b4d2a162e0.json
# 0199e3da-fcff-7543-a657-1d721c6712d4.json
```

**Indexes** in Redis:
```bash
redis-cli SMEMBERS "coffees-by-type:Espresso"
# "coffees/0199e3da-fcf9-762e-821e-57b4d2a162e0.json"
# "coffees/0199e3da-fcff-7543-a657-1d721c6712d4.json"
```

Best of both worlds: **Human-readable files** + **Fast Redis queries**.

## Modify the Example

Try tracking different coffees:

```go
coffees.Create(ctx, &Coffee{Type: "Latte", Size: "Large"})
coffees.Create(ctx, &Coffee{Type: "Americano", Size: "Medium"})

lattes, _ := coffees.Find(ctx, "type", "Latte")
fmt.Printf("Lattes consumed: %d\n", len(lattes))
```

The indexes just work. No schema changes needed.

## Next Steps

- See [02-simple-crud](../02-simple-crud) for full CRUD operations (Update, Delete, Get by ID)
- See [03-with-indexing](../03-with-indexing) for unique indexes and more advanced patterns

## Requirements

- **Redis or Valkey** must be running (indexes require it)
- Default: `localhost:6379` (override with `REDIS_ADDR` env var)
