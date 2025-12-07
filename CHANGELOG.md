## [1.2.0](https://github.com/adrianmcphee/smarterbase/compare/v1.1.0...v1.2.0) (2025-12-07)

### Features

* add Django ORM and Laravel/Eloquent to driver terminal ([8805466](https://github.com/adrianmcphee/smarterbase/commit/8805466db21b4e43a68fef0e417dac087414994b))

## [1.1.0](https://github.com/adrianmcphee/smarterbase/compare/v1.0.1...v1.1.0) (2025-12-07)

### Features

* cycle languages in driver terminal with 100 chaotic migrations ([a2c1922](https://github.com/adrianmcphee/smarterbase/commit/a2c19220e997f29d94564f42eb503cfeea1c36b5))

## [1.0.1](https://github.com/adrianmcphee/smarterbase/compare/v1.0.0...v1.0.1) (2025-12-07)

### Bug Fixes

* update version string and error handling for v1.0.0 ([e5d0c1c](https://github.com/adrianmcphee/smarterbase/commit/e5d0c1c0ec451336626a72d63a9ad7c667f1e349))

# Changelog

All notable changes to SmarterBase will be documented in this file.

## [1.0.0] - 2025-12-07

Initial release of SmarterBase as a PostgreSQL wire protocol server.

### Features

- **PostgreSQL wire protocol** - Connect with any PostgreSQL driver (psql, SQLAlchemy, ActiveRecord, Prisma, GORM, etc.)
- **JSON file storage** - Schema as JSON, data as JSONL (one file per table)
- **SQL support** - CREATE TABLE, CREATE INDEX, SELECT, INSERT, UPDATE, DELETE
- **WHERE clauses** - =, <, >, IN, LIKE operators
- **ORDER BY, LIMIT, OFFSET** - Full pagination support
- **UUIDv7 primary keys** - Time-ordered, generated with `gen_random_uuid7()`
- **Export to PostgreSQL** - `smarterbase export` generates clean DDL + INSERT statements

### Philosophy

SmarterBase separates exploration from production:

- During development: Schema is JSON files, AI assistants edit directly, no migrations accumulate
- When ready: Export to PostgreSQL with a clean schema, not 100 exploratory migrations

### Out of Scope (by design)

These features require database internals we intentionally don't build:

- Transactions (requires WAL)
- JOINs (query each table, join in app)
- Aggregations (COUNT/SUM in app code)
- Subqueries
- Replication
