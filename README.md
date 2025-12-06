# SmarterBase

**AI-powered development database. PostgreSQL compatibility. NVMe speed.**

An intelligent development database that helps you define your data model, generates realistic synthetic data, and speaks PostgreSQL wire protocol. Iterate freely with JSON files you can see and edit. When your schema stabilizes, export to PostgreSQL.

Perfect for **early development**, **prototypes**, **demos**, and **single-server production**.

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

---

## The Problem

Early development is painful:

1. **You don't know your data model yet** - But databases force you to decide upfront and accumulate migrations.
2. **You need realistic test data** - But creating it manually is tedious and unrealistic.
3. **Your data is opaque** - You can't just `cat` a record or `grep` for a value.
4. **SQLite isn't PostgreSQL** - Different dialect means rewriting queries later.

What if an AI could help you design your schema, generate realistic data, and give you a PostgreSQL-compatible database that stores everything as visible JSON files?

## The Solution

SmarterBase is an AI-powered development database. Connect your LLM provider, describe your domain, and get:

```
┌─────────────────────────────────────────────────────────────┐
│                      smarterbase                             │
│                                                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌─────────────┐  │
│  │    AI    │  │ pgproto3 │  │ sqlparser│  │   storage   │  │
│  │ (schema  │  │(protocol)│  │ (parse)  │  │ (files+idx) │  │
│  │  + data) │  │          │  │          │  │             │  │
│  └──────────┘  └──────────┘  └──────────┘  └─────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

**AI-assisted schema design:** Describe your domain, get schema suggestions.

```bash
smarterbase design
> Describe your project: B2B SaaS for project management
> AI suggests: companies, users, projects, tasks, comments...
> Refine interactively until your model feels right
```

**Realistic synthetic data:** AI generates data that makes sense for your domain.

```bash
smarterbase generate --count 100
> Generating realistic B2B SaaS data...
> Companies: Acme Corp, TechStart Inc, GlobalSoft...
> Users: realistic names, emails, roles per company...
> Projects: believable names, descriptions, statuses...
```

**Reusable data generators:** Not just one-time data - generators you can run anytime.

**PostgreSQL wire protocol:** Your app connects with any PostgreSQL driver. Real queries, real testing.

**See everything:** Your data is JSON files. `cat`, `grep`, `git diff` your records.

**Fast:** Local NVMe means point lookups under 100μs.

| Operation | Typical Latency |
|-----------|-----------------|
| Local NVMe read | 10-100 μs |
| Redis over network | 500-2000 μs |
| PostgreSQL over network | 1-10 ms |

**Migrate when ready:** Export schema + data generators to PostgreSQL. Your queries already work.

---

## Quick Start

```bash
# Install
go install github.com/adrianmcphee/smarterbase/cmd/smarterbase@latest

# Configure your LLM provider (pick one)
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
export GEMINI_API_KEY="..."

# Design your schema with AI assistance
smarterbase design
> Describe your project: E-commerce marketplace
> AI suggests tables: users, products, orders, reviews...
> Refine until it feels right

# Generate realistic synthetic data
smarterbase generate --count 100

# Start the server
smarterbase serve --port 5433 --data ./data
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

### Local NVMe is fast

| Operation | Latency |
|-----------|---------|
| Local NVMe read | 10-100 μs |
| PostgreSQL (network) | 1-10 ms |

For `SELECT * FROM users WHERE id = $1`, reading a JSON file from NVMe is faster than a network round-trip to PostgreSQL.

### PostgreSQL protocol means zero app changes

Same code. Same queries. Different backend. Works with any PostgreSQL driver.

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

### Directory Structure

```
./data/
├── _schema/
│   └── users.json
├── _idx/
│   └── users/
│       ├── email.json          # {"alice@example.com": "019363e8-..."}
│       └── role/
│           ├── admin.json      # ["019363e8-...", "019363f2-..."]
│           └── user.json       # ["019363f5-..."]
├── users/
│   ├── 019363e8-7a6b-7def-8000-1a2b3c4d5e6f.json
│   └── 019363f2-8b7c-7abc-8000-2b3c4d5e6f7a.json
└── orders/
    └── 019363f5-9c8d-7bcd-8000-3c4d5e6f7a8b.json
```

Files are named by UUIDv7. Because UUIDv7 is time-ordered, `ls` shows documents in creation order.

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
# AI-assisted schema design
smarterbase design

# Generate realistic synthetic data
smarterbase generate --count 100

# Start server
smarterbase serve --port 5433 --data ./data

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

# LLM provider (pick one)
llm:
  provider: anthropic  # or: openai, gemini
  # API key from environment: ANTHROPIC_API_KEY, OPENAI_API_KEY, or GEMINI_API_KEY
```

Or use environment variables:

```bash
export ANTHROPIC_API_KEY="sk-ant-..."  # or OPENAI_API_KEY or GEMINI_API_KEY
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

### Use It For

- **Early development** - Explore your data model without touching production. No migration debt.
- **Prototypes & demos** - Self-contained, no database setup, just run the binary.
- **Single-server production** - The pattern works. NVMe is fast. Backups are just file copies.
- **Learning** - See your data as JSON files. Understand what's happening.

### Graduate to PostgreSQL When You Need

- **Transactions** - ACID guarantees across multiple operations
- **JOINs and aggregations** - Complex queries
- **Multi-server deployments** - Replication, clustering
- **More than ~1M rows/table** - Query planner benefits kick in

Export to PostgreSQL anytime. Your queries already work.

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

MIT License - See [LICENSE](./LICENSE) file for details
