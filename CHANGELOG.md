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
