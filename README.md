# SmarterBase

**Explore freely. Graduate cleanly.**

A PostgreSQL-compatible database that stores data as JSON files. AI assistants edit your schema directly. When you're ready for PostgreSQL, export just your final schema—not 100 exploratory migrations.

Built for **Claude Code**, **Cursor**, **Copilot**, and the AI-assisted development workflow.

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: BSL 1.1](https://img.shields.io/badge/License-BSL_1.1-blue.svg)](./LICENSE)

---

## The Problem

Early development is exploration. You don't know your schema yet.

But traditional databases treat every change as permanent:

1. **Each experiment becomes a migration file** — AI helps you iterate 10x faster, which means 10x more migrations. Your "figuring it out" phase becomes 100+ ALTER TABLEs everyone replays forever.
2. **AI assistants can't help effectively** — Claude Code could fix your schema in seconds—if it didn't have to generate migration SQL and coordinate versions.
3. **Your data is opaque** — You can't just `cat` a record or `grep` for a value without special tools.
4. **SQLite doesn't match production** — Different SQL dialect means rewriting queries when you graduate to PostgreSQL.

**What if exploration had no permanent cost?**

## The Solution

SmarterBase separates exploration from production.

During development, your schema is JSON files. AI assistants edit them directly. No migrations accumulate.

When you're ready, export to PostgreSQL—with a clean schema, not your exploratory history.

```
┌─────────────────────────────────────────────────────────────┐
│  EXPLORE (SmarterBase)             GRADUATE (PostgreSQL)    │
│                                                             │
│  • Schema is JSON files    ───▶    • Export final schema    │
│  • AI edits directly               • Clean starting point   │
│  • No migrations needed            • No migration history   │
│  • Iterate freely                  • Queries already work   │
└─────────────────────────────────────────────────────────────┘
```

**How AI assistants use SmarterBase:**

```bash
# Claude Code sees your entire data model
cat data/_schema/users.json data/users.jsonl

# Claude Code changes your schema directly
claude "add a role column to users"
# → edits data/_schema/users.json (no migration generated)

# Made a mistake? Just revert
git checkout data/_schema/users.json
```

**Schema is just JSON:**

```sql
CREATE TABLE users (id UUID PRIMARY KEY, email TEXT, name TEXT);
```

Creates `data/_schema/users.json`:
```json
{"name": "users", "columns": [
  {"name": "id", "type": "uuid", "primary_key": true},
  {"name": "email", "type": "text"},
  {"name": "name", "type": "text"}
]}
```

**PostgreSQL wire protocol:** Any pg driver works. Same code runs against PostgreSQL when you graduate.

**See everything:** `cat`, `grep`, `git diff` your data.

---

## Quick Start

```bash
# Install
go install github.com/adrianmcphee/smarterbase/cmd/smarterbase@latest

# Start the server
smarterbase --port 5433 --data ./data
```

```bash
# Connect with psql
psql -h localhost -p 5433

# Create tables with standard SQL
CREATE TABLE users (id UUID PRIMARY KEY, email TEXT, name TEXT);
INSERT INTO users (id, email, name) VALUES (gen_random_uuid7(), 'alice@example.com', 'Alice');
SELECT * FROM users;
```

Connect from any language:

```python
# Python / SQLAlchemy
DATABASE_URL = "postgresql://localhost:5433/myapp"
```

```javascript
// Node.js
const pool = new Pool({ host: 'localhost', port: 5433 });
```

```ruby
# Ruby / Rails
host: localhost
port: 5433
```

```go
// Go
db, _ := sql.Open("postgres", "host=localhost port=5433 dbname=myapp sslmode=disable")
```

---

## Why This Works

**AI coding assistants love editable files.** Claude Code, Cursor, Copilot—they can all edit JSON files directly. No need to generate migration SQL, coordinate versions, or run migration commands.

**Local NVMe is fast.** For `SELECT * FROM users WHERE id = $1`, reading a JSON file is faster than a network round-trip:

| Operation | Latency |
|-----------|---------|
| Local NVMe read | 10-100 μs |
| PostgreSQL (network) | 1-10 ms |

**PostgreSQL protocol means zero app changes.** Same code, same queries. When you're ready for production, just change your connection string.

---

## Features

### In Scope

| Feature | Description |
|---------|-------------|
| Single-table CRUD | SELECT, INSERT, UPDATE, DELETE |
| WHERE clauses | =, <, >, IN, LIKE |
| ORDER BY, LIMIT, OFFSET | Pagination |
| CREATE TABLE, CREATE INDEX | Schema definition |
| UUIDv7 primary keys | Time-ordered, PostgreSQL-native |
| JSON file storage | Human-readable, debuggable |
| Export to PostgreSQL | The escape hatch |

### Out of Scope

| Feature | Rationale |
|---------|-----------|
| Transactions | Requires WAL. Use PostgreSQL. |
| JOINs | Query each table, join in app |
| Aggregations | COUNT/SUM in app code |
| Subqueries | Complexity for rare use case |
| Replication | Single server only |

**The rule:** If it requires building database internals (query planner, WAL, MVCC), it's out of scope.

---

## How It Works

### Architecture

```
┌─────────────────────────────────────────────────┐
│                  smarterbase                    │
│                                                 │
│  ┌───────────┐  ┌──────────┐  ┌─────────────┐   │
│  │ protocol  │─▶│  parser  │─▶│   storage   │   │
│  │ (pg wire) │  │  (SQL)   │  │ (files+idx) │   │
│  └───────────┘  └──────────┘  └─────────────┘   │
│                                                 │
└─────────────────────────────────────────────────┘
```

Three components:
1. **Protocol** - PostgreSQL wire protocol (any pg driver works)
2. **Parser** - SQL to AST
3. **Storage** - JSON files + JSON indexes

No query planner. No optimizer. Parse SQL, execute against files, return results.

### Directory Structure

```
./data/
├── _schema/
│   └── users.json              # schema definition
└── users.jsonl                 # all rows in one file
```

---

## LLM-Friendly Storage (JSONL)

SmarterBase uses **JSONL (JSON Lines)** format—one file per table, one JSON object per line:

```jsonl
# data/users.jsonl
{"id":"u1","name":"Alice","email":"alice@example.com"}
{"id":"u2","name":"Bob","email":"bob@example.com"}
```

**Why this matters for AI-assisted development:**

1. **Full table context** - LLMs see your entire table in one `cat` command
2. **Schema + data together** - `cat data/_schema/users.json data/users.jsonl` gives complete picture
3. **Easy editing** - No migrations, just edit JSON files directly
4. **Git-friendly** - Track schema and data changes with version control
5. **Standard format** - JSONL is used by OpenAI, BigQuery, and many data tools

---

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
- **Time-ordered** - IDs sort chronologically. No need for `created_at` index.
- **No coordination** - Generate IDs anywhere without a central authority.
- **PostgreSQL-native** - PostgreSQL 17+ supports UUIDv7. Migration is seamless.
- **Filesystem-friendly** - Lexicographic sort = chronological sort.

---

## SQL Examples

```sql
-- Data Definition
CREATE TABLE users (id UUID PRIMARY KEY, email TEXT UNIQUE, name TEXT)
CREATE INDEX idx_role ON users(role)

-- Queries
SELECT * FROM users WHERE id = $1
SELECT * FROM users WHERE email = $1
SELECT * FROM users WHERE role = 'admin' ORDER BY id DESC LIMIT 10

-- Mutations
INSERT INTO users (email, name) VALUES ($1, $2)  -- auto-generates id
UPDATE users SET name = $1 WHERE id = $2
DELETE FROM users WHERE id = $1
```

Note: `ORDER BY id DESC` gives you most-recent-first because UUIDv7 is time-ordered.

---

## Migration to PostgreSQL

When you outgrow smarterbase:

```bash
smarterbase export > dump.sql
psql myapp < dump.sql
```

Update your database config to point to PostgreSQL. Done.

The export generates:
- `CREATE TABLE` statements with proper UUID types
- `INSERT` statements with all data
- `CREATE INDEX` statements

UUIDv7 values transfer directly - PostgreSQL's UUID type accepts them as-is.

---

## CLI

```bash
# Start server
smarterbase --port 5433 --data ./data

# Export to PostgreSQL format
smarterbase export > dump.sql

# Rebuild indexes after crash
smarterbase rebuild-indexes
```

---

## Configuration

```yaml
# smarterbase.yaml (optional)
port: 5433
data: ./data
password: ""  # empty = no auth
```

Or use command-line flags:

```bash
smarterbase --port 5433 --data ./data
```

---

## Consistency Model

**Document writes are atomic.** Temp file + rename ensures a document is either fully written or not written.

**Index updates are best-effort.** If you crash between writing a document and updating its indexes, the indexes may be stale.

**Recovery:**

```bash
smarterbase rebuild-indexes
```

This scans all documents and rebuilds all indexes. Run it if you suspect index drift after a crash.

If you need crash-consistent indexes, use PostgreSQL.

---

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

---

## ORM/Framework Compatibility

ORMs and migration tools probe the database on startup. We implement minimum pg_catalog:

```sql
SELECT * FROM pg_tables WHERE schemaname = 'public'
SELECT * FROM information_schema.columns WHERE table_name = $1
```

**Tested frameworks:**
- Python: SQLAlchemy, Django ORM, Alembic migrations
- Ruby: ActiveRecord, Rails migrations
- Node.js: Prisma, Knex, TypeORM
- Go: GORM, sqlx
- PHP: Laravel Eloquent, Doctrine

---

## When to Use SmarterBase

### The Exploration Phase

Use SmarterBase when you're still figuring out your data model:

| You're doing this... | SmarterBase helps because... |
|---------------------|------------------------------|
| AI-assisted development | Claude/Cursor/Copilot edit schema JSON directly |
| Rapid prototyping | Schema changes are instant, no migration files |
| Learning & experimenting | See your data as JSON files, understand what's happening |
| Building demos | Self-contained, no database setup required |

### The Graduation

When your schema stabilizes and you need production features:

```bash
# Export your final schema (not 100 migrations)
smarterbase export > schema.sql

# Load into PostgreSQL
psql myapp < schema.sql

# Update connection string. Done.
# Your queries already work—same PostgreSQL dialect.
```

**What you bring:** Your final schema.
**What you leave behind:** Migration history, ALTER TABLE archaeology, experimental baggage.

### Graduate to PostgreSQL When You Need

- **Transactions** - ACID guarantees across multiple operations
- **JOINs and aggregations** - Complex queries
- **Multi-server deployments** - Replication, clustering
- **More than ~1M rows/table** - Query planner benefits kick in

---

## Documentation

- [RFC-0001: Filesystem-Native Storage with PostgreSQL Wire Protocol](./docs/rfc/0001-filesystem-native-postgres-protocol.md)
- [ADR-0001: PostgreSQL Wire Protocol Over Filesystem Storage](./docs/adr/0001-postgresql-wire-protocol-over-filesystem.md)

---

## Contributing

Contributions welcome! Please ensure:
- Tests pass: `go test -v -race`
- Code is formatted: `go fmt`

---

## License

Business Source License 1.1 - See [LICENSE](./LICENSE) for details.

**Free to use** for internal/personal use, education, and building apps that connect to SmarterBase.

**Commercial license required** for offering SmarterBase as a managed service or building competing products. Contact license@smarterbase.com.

Converts to MIT License 4 years after each release.
