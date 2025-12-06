## [4.0.5](https://github.com/adrianmcphee/smarterbase/compare/v4.0.4...v4.0.5) (2025-12-06)

### Bug Fixes

* use HTML entities for logo block characters ([b0d386e](https://github.com/adrianmcphee/smarterbase/commit/b0d386e01870e792bce94826cefb1fd530807249))

## [4.0.4](https://github.com/adrianmcphee/smarterbase/compare/v4.0.3...v4.0.4) (2025-12-06)

### Bug Fixes

* use more visible terracotta color for logo ([33e1307](https://github.com/adrianmcphee/smarterbase/commit/33e1307ab59834697f3c30dcd18380de63daff65))

## [4.0.3](https://github.com/adrianmcphee/smarterbase/compare/v4.0.2...v4.0.3) (2025-12-06)

### Bug Fixes

* use correct quadrant block characters for logo ([c07acbe](https://github.com/adrianmcphee/smarterbase/commit/c07acbe17fc1732af82e2e72a28a0b1f18ab3b80))

## [4.0.2](https://github.com/adrianmcphee/smarterbase/compare/v4.0.1...v4.0.2) (2025-12-06)

### Bug Fixes

* update terminal logo to pixel art style ([c69871d](https://github.com/adrianmcphee/smarterbase/commit/c69871d338900211ac184ee45171fd6756c159b5))

## [4.0.1](https://github.com/adrianmcphee/smarterbase/compare/v4.0.0...v4.0.1) (2025-12-06)

### Bug Fixes

* terminal animation comment/prompt line break and logo ([1388315](https://github.com/adrianmcphee/smarterbase/commit/13883158522c62f736a12c11c4afe7ee85a0f82d))

### Documentation

* add smarterbase.com to site URLs ([854c65d](https://github.com/adrianmcphee/smarterbase/commit/854c65df2bfde3a5fab50e75a5786eb1b6de537c))
* reframe value prop around exploration and clean graduation ([1a6d308](https://github.com/adrianmcphee/smarterbase/commit/1a6d3081dcd4886923dadda2db28384aff2b8d2e))
* update hero terminal to show schema editing workflow ([20316f5](https://github.com/adrianmcphee/smarterbase/commit/20316f508dbcc7f5eaeac4b1dc127c2b5063ed2a))

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
