# Changelog

All notable changes to SmarterBase will be documented in this file.

## [4.0.0] - 2025-12-06

### Changed

- **Remove old Redis+S3 library code** - SmarterBase is now purely a PostgreSQL wire protocol server
- Clean up dependencies (removed ~50 packages, kept 4)
- Simplified CI workflow (removed Redis/MinIO services)
- Updated documentation (SECURITY.md, CONTRIBUTING.md, Makefile)

### Removed

- All root-level Go library files (store, backend, redis, s3, encryption, metrics, etc.)
- `examples/` directory
- `simple/` package
- `DATASHEET.md`
- `scripts/` directory

## [3.6.0] - 2025-12-06

### Features

- Redesigned website with dark theme and animated terminals
- Fixed height terminals with auto-scroll

## [3.5.0] - 2025-12-06

### Features

- Add animated terminal demos showing PostgreSQL driver usage
- Multi-language examples (Python, Go, Ruby, Node.js)

## [3.4.0] - 2025-12-06

### Features

- Implement JSONL storage format (one file per table, one JSON per line)

## [3.3.0] - 2025-12-06

### Features

- Implement working SQL executor with JSONL storage

## [3.0.0] - 2025-12-06

### Breaking Changes

Complete architecture pivot from Redis+S3 to PostgreSQL wire protocol.

**New architecture:**
- PostgreSQL wire protocol via jackc/pgproto3
- SQL parsing via vitess/sqlparser
- JSONL file storage with atomic writes
- UUIDv7 primary keys (time-ordered)

**Works with any PostgreSQL driver:**
- Python: SQLAlchemy, Django
- Ruby: ActiveRecord, Rails
- Node.js: Prisma, Knex
- Go: GORM, sqlx

**Scope:**
- Single-table CRUD (SELECT, INSERT, UPDATE, DELETE)
- WHERE with =, <, >, IN, LIKE
- ORDER BY, LIMIT, OFFSET
- CREATE TABLE, CREATE INDEX
- Export to PostgreSQL

**Out of scope (use PostgreSQL):**
- Transactions, JOINs, aggregations, subqueries, replication
