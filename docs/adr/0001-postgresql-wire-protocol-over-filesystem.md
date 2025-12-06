# ADR-0001: PostgreSQL Wire Protocol Over Filesystem Storage

**Status:** Accepted
**Date:** 2024-12-06
**Related RFC:** [RFC-0001](../rfc/0001-filesystem-native-postgres-protocol.md)

## Context

Building a new application requires database setup. PostgreSQL requires running a server. SQLite has a different SQL dialect that makes migration painful.

We want:
- Zero setup for prototypes
- Standard PostgreSQL compatibility
- Human-readable data files
- Clear path to PostgreSQL when ready

## Decision

Build a PostgreSQL wire protocol server that stores data as JSON files on the local filesystem.

**Key choices:**

1. **PostgreSQL wire protocol** - Applications connect using standard pg drivers. No client libraries to build or maintain.

2. **JSON file storage** - One file per row, human-readable, debuggable with `cat` and `grep`.

3. **JSON indexes** - Simple MapIndex (unique) and ListIndex (1:N) stored as JSON files.

4. **UUIDv7 primary keys** - Time-ordered, no coordination required, PostgreSQL-native.

5. **ActiveRecord-first** - Target one ORM, make it work perfectly.

6. **Explicit limitations** - No transactions, no JOINs, no aggregations. When you need these, migrate to PostgreSQL.

## Consequences

### Positive

- Zero external dependencies (single Go binary)
- Local NVMe is 10-50x faster than network database calls
- Data is human-readable and git-friendly
- Any PostgreSQL client library works
- Migration to PostgreSQL = export and change config

### Negative

- Single server only
- Best-effort index consistency (rebuild after crash)
- Limited SQL subset
- ~1M rows/table practical limit

### Neutral

- Target audience is prototypes and small applications
- Not competing with PostgreSQL - it's a stepping stone to PostgreSQL

## Implementation

See [RFC-0001](../rfc/0001-filesystem-native-postgres-protocol.md) for technical specification.

```
cmd/smarterbase/       - CLI
internal/protocol/     - PostgreSQL wire protocol
internal/storage/      - Filesystem backend
internal/index/        - MapIndex, ListIndex
internal/executor/     - Query execution
internal/catalog/      - Schema, pg_catalog emulation
```
