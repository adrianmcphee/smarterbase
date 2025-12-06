# RFC-0001: Filesystem-Native Storage with PostgreSQL Wire Protocol

**Status:** Accepted
**Authors:** Adrian McPhee
**Created:** 2024-12-06

## Summary

A PostgreSQL-compatible server that stores data as JSON files. Connect with standard PostgreSQL drivers, but your data lives in readable files on disk. When you outgrow it, migrate to real PostgreSQL.

## The Problem

Starting a new project requires either:

1. **Run PostgreSQL** - Docker, Homebrew, or managed service. Overhead for a prototype.
2. **Use SQLite** - Different SQL dialect. Migration to PostgreSQL means rewriting queries.

Neither option gives you: zero setup now, seamless PostgreSQL migration later.

## The Solution

Speak PostgreSQL wire protocol, store data as JSON files.

```
Your App (any PostgreSQL driver)
        │
        │ PostgreSQL wire protocol
        ▼
   smarterbase
        │
        ▼
   JSON files on disk
```

Your app thinks it's talking to PostgreSQL. Your data lives in files you can read, debug, and git commit. When you need real PostgreSQL, export and switch.

## Why This Works

**Local NVMe is fast:**

| Operation | Latency |
|-----------|---------|
| Local NVMe read | 10-100 μs |
| PostgreSQL (network) | 1-10 ms |

For `SELECT * FROM users WHERE id = $1`, reading a JSON file from NVMe is faster than a network round-trip to PostgreSQL.

**PostgreSQL protocol means zero app changes:**

```python
# Python/SQLAlchemy/Django
DATABASE_URL = "postgresql://localhost:5433/myapp"  # smarterbase
DATABASE_URL = "postgresql://prod-db:5432/myapp"    # real PostgreSQL
```

```javascript
// Node.js
const pool = new Pool({ host: 'localhost', port: 5433 });  // smarterbase
const pool = new Pool({ host: 'prod-db', port: 5432 });    // real PostgreSQL
```

```ruby
# Ruby/Rails
host: localhost
port: 5433  # smarterbase → port: 5432 for real PostgreSQL
```

Same code. Same queries. Different backend. Works with any PostgreSQL driver.

## Scope

### In Scope

| Feature | Rationale |
|---------|-----------|
| Single-table CRUD | Covers 90% of prototype needs |
| WHERE with =, <, >, IN, LIKE | Basic filtering |
| ORDER BY, LIMIT, OFFSET | Pagination |
| CREATE TABLE, CREATE INDEX | Schema definition |
| UUIDv7 primary keys | Time-ordered, PostgreSQL-native, migration-friendly |
| Standard PostgreSQL drivers | Any language: Python, Ruby, Node.js, Go, PHP, Java |
| JSON file storage | Human-readable, debuggable |
| Export to PostgreSQL | The escape hatch |

### Out of Scope

| Feature | Rationale |
|---------|-----------|
| Transactions | Requires write-ahead logging. Complexity explosion. Use PostgreSQL. |
| JOINs | Query each table, join in app. Keeps executor simple. |
| Aggregations | COUNT/SUM in app. No query planner needed. |
| Subqueries | Complexity for rare use case. |
| Replication | Single server only. Use PostgreSQL for multi-server. |

The rule: if it requires building database internals (query planner, WAL, MVCC), it's out of scope. Use PostgreSQL.

## Architecture

```
┌─────────────────────────────────────────────────┐
│                  smarterbase                    │
│                                                 │
│  ┌───────────┐  ┌──────────┐  ┌─────────────┐   │
│  │ pgproto3  │─▶│ sqlparser│─▶│   storage   │   │
│  │ (protocol)│  │ (parse)  │  │ (files+idx) │   │
│  └───────────┘  └──────────┘  └─────────────┘   │
│                                                 │
└─────────────────────────────────────────────────┘
```

Three components:

1. **Protocol** - `jackc/pgproto3` handles PostgreSQL wire protocol
2. **Parser** - `vitess/sqlparser` parses SQL to AST
3. **Storage** - JSON files + JSON indexes

No query planner. No optimizer. Parse SQL, execute against files, return results.

## UUIDv7 Primary Keys

All tables use UUIDv7 as the default primary key type:

```sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid7(),
    email TEXT UNIQUE,
    name TEXT
)
```

**Why UUIDv7:**

- **Time-ordered** - UUIDv7 encodes timestamp in the first 48 bits. IDs sort chronologically. No need for `created_at` index for "most recent" queries.
- **No coordination** - Generate IDs anywhere without a sequence or central authority.
- **PostgreSQL-native** - PostgreSQL 17+ supports UUIDv7 via `pg_uuidv7` extension. Migration is seamless.
- **Filesystem-friendly** - Lexicographic sort = chronological sort. Listing files shows natural order.

**Structure:**

```
019363e8-7a6b-7def-8000-1a2b3c4d5e6f
│        │    │
│        │    └── version 7 marker
│        └── random bits
└── millisecond timestamp (48 bits)
```

**Generation:**

```go
func NewUUIDv7() string {
    now := time.Now().UnixMilli()

    var uuid [16]byte
    binary.BigEndian.PutUint48(uuid[0:6], uint64(now))
    rand.Read(uuid[6:16])

    uuid[6] = (uuid[6] & 0x0F) | 0x70  // version 7
    uuid[8] = (uuid[8] & 0x3F) | 0x80  // variant 2

    return formatUUID(uuid)
}
```

## Implementation

### Query Execution

```go
func (s *Server) Execute(ctx context.Context, sql string) ([]Row, error) {
    stmt, _ := sqlparser.Parse(sql)

    switch q := stmt.(type) {
    case *sqlparser.Select:
        return s.execSelect(ctx, q)
    case *sqlparser.Insert:
        return s.execInsert(ctx, q)
    case *sqlparser.Update:
        return s.execUpdate(ctx, q)
    case *sqlparser.Delete:
        return s.execDelete(ctx, q)
    }
    return nil, ErrNotSupported
}

func (s *Server) execSelect(ctx context.Context, q *sqlparser.Select) ([]Row, error) {
    table := extractTable(q.From)
    where := extractWhere(q.Where)

    // Index lookup if possible
    if where.IsEquality() {
        if idx := s.indexes.Get(table, where.Field); idx != nil {
            id := idx.Lookup(where.Value)
            doc, _ := s.storage.Get(ctx, table, id)
            return []Row{doc}, nil
        }
    }

    // Otherwise scan
    return s.storage.Scan(ctx, table, where.Filter)
}
```

### Storage (JSONL)

```go
type DataStore struct {
    dataDir string
    schema  *SchemaStore
    mu      sync.RWMutex
}

// tablePath returns path to table's JSONL file
func (d *DataStore) tablePath(tableName string) string {
    return filepath.Join(d.dataDir, tableName+".jsonl")
}

// readAllRows reads all rows from a table's JSONL file
func (d *DataStore) readAllRows(tableName string) ([]Row, error) {
    path := d.tablePath(tableName)
    file, _ := os.Open(path)
    defer file.Close()

    var rows []Row
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        var row Row
        json.Unmarshal(scanner.Bytes(), &row)
        rows = append(rows, row)
    }
    return rows, nil
}

// writeAllRows writes all rows atomically (temp file + rename)
func (d *DataStore) writeAllRows(tableName string, rows []Row) error {
    path := d.tablePath(tableName)
    tmp := path + ".tmp"

    file, _ := os.Create(tmp)
    writer := bufio.NewWriter(file)
    for _, row := range rows {
        data, _ := json.Marshal(row)
        writer.Write(data)
        writer.WriteString("\n")
    }
    writer.Flush()
    file.Close()

    return os.Rename(tmp, path)  // Atomic!
}
```

JSONL format: one file per table, one JSON object per line. LLM-friendly - full table context in one `cat`.

### Indexes

```go
// MapIndex: unique lookups (email → id)
type MapIndex struct {
    path string
    data map[string]string
}

func (m *MapIndex) Lookup(key string) string {
    if m.data == nil {
        data, _ := os.ReadFile(m.path)
        json.Unmarshal(data, &m.data)
    }
    return m.data[key]
}

// ListIndex: non-unique lookups (role → [ids])
type ListIndex struct {
    baseDir string
}

func (l *ListIndex) Lookup(key string) []string {
    path := filepath.Join(l.baseDir, key+".json")
    data, _ := os.ReadFile(path)
    var ids []string
    json.Unmarshal(data, &ids)
    return ids
}
```

JSON indexes. Simple, readable, debuggable.

### Directory Structure

```
./data/
├── _schema/
│   └── users.json              # {"name":"users","columns":[...]}
├── _idx/
│   └── users/
│       ├── email.json          # {"alice@example.com": "019363e8-..."}
│       └── role/
│           ├── admin.json      # ["019363e8-...", "019363f2-..."]
│           └── user.json       # ["019363f5-..."]
├── users.jsonl                 # All user rows, one JSON per line
└── orders.jsonl                # All order rows, one JSON per line
```

Each table is stored as a JSONL (JSON Lines) file. Each line is one row as JSON. This is LLM-friendly - one `cat` shows the entire table.

## Consistency Model

**Document writes are atomic.** Temp file + rename ensures a document is either fully written or not written.

**Index updates are best-effort.** If you crash between writing a document and updating its indexes, the indexes may be stale.

**Recovery:**

```bash
smarterbase rebuild-indexes
```

This scans all documents and rebuilds all indexes. Run it if you suspect index drift after a crash.

This is acceptable because:
1. Crashes during writes are rare
2. This is a development/prototype database
3. The fix is one command

If you need crash-consistent indexes, use PostgreSQL.

## Supported SQL

```sql
-- Data Definition
CREATE TABLE users (id UUID PRIMARY KEY, email TEXT UNIQUE, name TEXT)
CREATE INDEX idx_role ON users(role)

-- Queries
SELECT * FROM users WHERE id = $1
SELECT * FROM users WHERE email = $1
SELECT * FROM users WHERE role = 'admin' ORDER BY id DESC LIMIT 10

-- Mutations
INSERT INTO users (id, email, name) VALUES (gen_random_uuid7(), $1, $2)
INSERT INTO users (email, name) VALUES ($1, $2)  -- auto-generates id
UPDATE users SET name = $1 WHERE id = $2
DELETE FROM users WHERE id = $1
```

Note: `ORDER BY id DESC` gives you most-recent-first because UUIDv7 is time-ordered.

## Not Supported

```sql
SELECT * FROM users u JOIN orders o ON o.user_id = u.id  -- No JOINs
SELECT COUNT(*) FROM users WHERE role = 'admin'          -- No aggregations
SELECT * FROM users WHERE id IN (SELECT user_id FROM ...)-- No subqueries
BEGIN; INSERT ...; INSERT ...; COMMIT;                   -- No transactions
```

If you need these, you need PostgreSQL.

## ORM/Framework Compatibility

ORMs and migration tools probe the database on startup. We implement minimum pg_catalog:

```sql
SELECT * FROM pg_tables WHERE schemaname = 'public'
SELECT * FROM information_schema.columns WHERE table_name = $1
```

These return data from our `_schema/` directory.

**Tested frameworks:**
- Python: SQLAlchemy, Django ORM, Alembic migrations
- Ruby: ActiveRecord, Rails migrations
- Node.js: Prisma, Knex, TypeORM
- Go: GORM, sqlx
- PHP: Laravel Eloquent, Doctrine

**Pre-implementation requirement:** Spike connectivity with target frameworks before building. Run with query logging, document exactly which queries are sent, confirm they're handleable.

## CLI

```bash
# Start server
smarterbase serve

# With options
smarterbase serve --port 5433 --data ./data

# Export to PostgreSQL format
smarterbase export > dump.sql

# Rebuild indexes after crash
smarterbase rebuild-indexes
```

## Configuration

```yaml
# smarterbase.yaml (optional)
port: 5433
data: ./data
password: ""  # empty = no auth
```

Defaults work. Config is optional.

## Migration to PostgreSQL

```bash
smarterbase export > dump.sql
psql myapp < dump.sql
```

The export generates:
- `CREATE TABLE` statements with proper UUID types
- `INSERT` statements with all data
- `CREATE INDEX` statements

UUIDv7 values transfer directly - PostgreSQL's UUID type accepts them as-is.

Update your database config to point to PostgreSQL. Done.

## Limitations

| Limitation | Implication |
|------------|-------------|
| No transactions | Crash between two INSERTs = partial state |
| No JOINs | Query tables separately, join in app |
| No aggregations | COUNT/SUM/AVG in app code |
| Single server | No replication, no clustering |
| ~1M rows/table | Beyond this, migrate to PostgreSQL |
| Best-effort indexes | Run `rebuild-indexes` after crash |

These are intentional. Keeping scope small keeps implementation simple.

## Implementation Plan

### Phase 1: MVP

- PostgreSQL wire protocol (simple query)
- SELECT, INSERT, UPDATE, DELETE
- UUIDv7 generation
- Filesystem storage with atomic writes
- MapIndex and ListIndex
- Minimum pg_catalog for ORMs
- `smarterbase serve`
- `smarterbase rebuild-indexes`

**Done when:** Any PostgreSQL client connects, runs migrations, does CRUD.

### Phase 2: Complete

- CREATE TABLE / CREATE INDEX
- ORDER BY, LIMIT, OFFSET
- WHERE operators: =, <, >, <=, >=, IN, LIKE, IS NULL
- Password authentication
- `smarterbase export`

**Done when:** You can build a real prototype.

## Success Criteria

1. Any PostgreSQL client connects and runs migrations
2. CRUD operations work from any language
3. Point lookups < 100μs on NVMe
4. Migration to PostgreSQL < 5 minutes
5. UUIDv7 IDs sort chronologically

## Dependencies

| Dependency | Purpose |
|------------|---------|
| [jackc/pgproto3](https://github.com/jackc/pgproto3) | PostgreSQL wire protocol |
| [vitess/sqlparser](https://github.com/vitessio/vitess) | SQL parsing |

Both are mature, well-maintained Go libraries.

## FAQ

**Why UUIDv7 instead of auto-increment?**

Auto-increment requires coordination (sequences). UUIDv7 can be generated anywhere without talking to the database. It's also time-ordered, so you get chronological sorting for free. PostgreSQL 17+ supports it natively, so migration is seamless.

**Why not SQLite?**

Different SQL dialect. Most ORMs generate PostgreSQL-specific migrations. Switching to PostgreSQL later requires query rewrites.

**Why not just use PostgreSQL?**

You can. This is for prototypes where you want zero setup and debuggable files.

**What about data safety?**

Individual writes are atomic. Index updates are best-effort. If you crash between writing a document and its indexes, run `rebuild-indexes`. If this isn't acceptable, use PostgreSQL.

**Can I use this in production?**

For a small internal tool on one server, sure. For customer-facing production, use PostgreSQL.
