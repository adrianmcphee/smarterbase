## [3.5.0](https://github.com/adrianmcphee/smarterbase/compare/v3.4.0...v3.5.0) (2025-12-06)

### Features

* add animated terminal demo with ASCII art logo ([305832f](https://github.com/adrianmcphee/smarterbase/commit/305832fc5baf25ec4f7a6907af1dcfa286463882))
* add second terminal demo showing multi-language PostgreSQL drivers ([6b11496](https://github.com/adrianmcphee/smarterbase/commit/6b11496a58ff07b5b113f1581ee8b8d5245be5cf))

### Bug Fixes

* add newline after language comments in driver terminal ([0b8ba78](https://github.com/adrianmcphee/smarterbase/commit/0b8ba784ce162425de15d5854bffe294082585ec))
* use simple ASCII box for logo (better font compatibility) ([9827823](https://github.com/adrianmcphee/smarterbase/commit/98278232311780dbe1ab47c45215f6050d795862))

### Documentation

* clarify migration debt pain point on website ([e076693](https://github.com/adrianmcphee/smarterbase/commit/e076693078f7965c94a12e322496408787bde78e))
* update RFC and ADR to reflect JSONL storage implementation ([2f8f65e](https://github.com/adrianmcphee/smarterbase/commit/2f8f65e03ac782e690bfad018d5bc13ff830ed57))

## [3.4.0](https://github.com/adrianmcphee/smarterbase/compare/v3.3.0...v3.4.0) (2025-12-06)

### Features

* implement JSONL storage format for LLM-friendly data ([5a1c3d2](https://github.com/adrianmcphee/smarterbase/commit/5a1c3d29ca3e414603eb0de6fb8dfebfa7ef2093))

## [3.3.0](https://github.com/adrianmcphee/smarterbase/compare/v3.2.0...v3.3.0) (2025-12-06)

### Features

* implement working SQL executor with LLM-friendly storage ([14581f7](https://github.com/adrianmcphee/smarterbase/commit/14581f746d9807dacb9b25076aa2f91cd8eab0a0))

## [3.2.0](https://github.com/adrianmcphee/smarterbase/compare/v3.1.0...v3.2.0) (2025-12-06)

### Features

* add pricing section and credit costs to website ([4c89590](https://github.com/adrianmcphee/smarterbase/commit/4c89590f7ac11d7b45b42e6e1f52596d0d01f624))

### Documentation

* remove library implementation details from user-facing content ([0647f85](https://github.com/adrianmcphee/smarterbase/commit/0647f85ec06ccbf4a3ea67d7fe449cac1c8c51ed))

## [3.1.0](https://github.com/adrianmcphee/smarterbase/compare/v3.0.0...v3.1.0) (2025-12-06)

### Features

* add schema design and synthetic data generation features ([9f799e0](https://github.com/adrianmcphee/smarterbase/commit/9f799e0cddaaae23677e12b3cd6d92931a421058))

### Documentation

* improve value proposition - iterate fast, lock in schema later ([0f7c00f](https://github.com/adrianmcphee/smarterbase/commit/0f7c00f382b1cca376b14390f0217ab076a9d2c8))
* refine value prop - early dev, prototypes, single-server production ([6a24112](https://github.com/adrianmcphee/smarterbase/commit/6a241124223a81cb38ed8ee1c4340ee323f0b279))

## [3.0.0](https://github.com/adrianmcphee/smarterbase/compare/v2.2.3...v3.0.0) (2025-12-06)

### ⚠ BREAKING CHANGES

* Complete architecture pivot from Redis+S3 to filesystem-native storage with PostgreSQL wire protocol.

New architecture:
- PostgreSQL wire protocol via jackc/pgproto3
- SQL parsing via vitess/sqlparser
- JSON file storage with atomic writes (temp file + rename)
- JSON indexes (MapIndex for unique, ListIndex for 1:N)
- UUIDv7 primary keys (time-ordered, PostgreSQL-native)

Language-agnostic - works with any PostgreSQL driver:
- Python: SQLAlchemy, Django, Alembic
- Ruby: ActiveRecord, Rails
- Node.js: Prisma, Knex, TypeORM
- Go: GORM, sqlx
- PHP: Laravel Eloquent, Doctrine

Scope:
- Single-table CRUD (SELECT, INSERT, UPDATE, DELETE)
- WHERE with =, <, >, IN, LIKE
- ORDER BY, LIMIT, OFFSET
- CREATE TABLE, CREATE INDEX
- Export to PostgreSQL for migration

Out of scope (use PostgreSQL):
- Transactions, JOINs, aggregations, subqueries, replication

Documentation changes:
- New RFC-0001: Filesystem-Native Storage with PostgreSQL Wire Protocol
- New ADR-0001: PostgreSQL Wire Protocol Over Filesystem Storage
- Removed old Redis+S3 ADRs (0001-0011)
- Rewrote README.md and website for new architecture

### Features

* pivot to PostgreSQL wire protocol over filesystem storage ([48f57b3](https://github.com/adrianmcphee/smarterbase/commit/48f57b37e0e857eaf6140d4999d9f27d3ee20e8d))

## [2.2.3](https://github.com/adrianmcphee/smarterbase/compare/v2.2.2...v2.2.3) (2025-11-23)

### Bug Fixes

* **indexer:** prevent cross-entity index pollution ([ec4eb98](https://github.com/adrianmcphee/smarterbase/commit/ec4eb983ebcdb8d02bc09a146121ab8f7f602c54))

## [2.2.2](https://github.com/adrianmcphee/smarterbase/compare/v2.2.1...v2.2.2) (2025-11-23)

### Bug Fixes

* make AutoRegisterIndexes strict - Redis is required, use miniredis in tests ([d57f6d0](https://github.com/adrianmcphee/smarterbase/commit/d57f6d0e9d8feaab693f73cffb1ec9be45e7ebe0))

## [2.2.1](https://github.com/adrianmcphee/smarterbase/compare/v2.2.0...v2.2.1) (2025-11-23)

### Bug Fixes

* make AutoRegisterIndexes graceful when redisIndexer is nil ([7055916](https://github.com/adrianmcphee/smarterbase/commit/70559164594deca68237869fbfe103d136a5c7a6))

### Documentation

* add ADR-0011 for Redis uniqueness constraints ([7464500](https://github.com/adrianmcphee/smarterbase/commit/746450070dfefe5b52bcc785d457cefc9ff8ccf1))
* fix ADR-0011 alternative option wording ([7012bf5](https://github.com/adrianmcphee/smarterbase/commit/7012bf5a3cb97fe4a24d1ab043d60f2169ec5bba))
* remove PostgreSQL alternative from ADR-0011 ([46297d8](https://github.com/adrianmcphee/smarterbase/commit/46297d8258fa839fd7392f1246de5e3e804dd7ff))

## [2.2.0](https://github.com/adrianmcphee/smarterbase/compare/v2.1.0...v2.2.0) (2025-11-23)

### Features

* add Redis-backed uniqueness constraints ([05e07c7](https://github.com/adrianmcphee/smarterbase/commit/05e07c7044334a7cab217a752a9691929ea5cb23))

## [2.1.0](https://github.com/adrianmcphee/smarterbase/compare/v2.0.2...v2.1.0) (2025-11-23)

### Features

* **redis:** add automatic TLS and SNI for managed Redis ([500fb42](https://github.com/adrianmcphee/smarterbase/commit/500fb42d5179f33eac55ee3fda5c04c0523f6359))

## [2.0.2](https://github.com/adrianmcphee/smarterbase/compare/v2.0.1...v2.0.2) (2025-11-22)

### Documentation

* update import paths to v2 in documentation ([6d26c37](https://github.com/adrianmcphee/smarterbase/commit/6d26c379384357827502d30a5dc7c3d97135590b))

### Code Refactoring

* remove legacy file-based indexing and improve error handling ([3901b86](https://github.com/adrianmcphee/smarterbase/commit/3901b863a876bde6a28ce4df14dc139c1ea05b93))

## [2.0.0](https://github.com/adrianmcphee/smarterbase/compare/v1.11.0...v2.0.0) (2025-11-16)

### ⚠ BREAKING CHANGES

* Remove file-based indexing in favor of Redis-only indexes

CONTEXT:
Smarterbase previously supported dual indexing (file-based + Redis).
This added complexity, slower performance, and filesystem clutter.
Redis is already required for rate limiting, sessions, and locks,
so graceful degradation provided no real benefit.

CHANGES:

Core Library:
- Delete indexer.go (120 lines) and indexer_test.go (291 lines)
- Remove Indexer type and file-based index support
- Update AutoRegisterIndexes() signature: remove fileIndexer parameter
  OLD: AutoRegisterIndexes(indexer, redisIndexer, entityType, example)
  NEW: AutoRegisterIndexes(redisIndexer, entityType, example)
- Update NewCascadeIndexManager() signature: remove indexer parameter
  OLD: NewCascadeIndexManager(base, indexer, redisIndexer)
  NEW: NewCascadeIndexManager(base, redisIndexer)
- Remove IndexManager.WithFileIndexer() method
- Reject sb:"index,unique" tags in ParseIndexTag()

Struct Tags:
- All indexes now use sb:"index" (Redis multi-value)
- Remove distinction between unique and multi-value indexes
- Application layer handles uniqueness constraints

Tests:
- Update auto_indexing_test.go for Redis-only testing
- Fix cascades_test.go to use new signatures
- Fix index_manager_test.go to remove file indexer usage
- Fix utility_functions_test.go to use MultiIndexSpec

Documentation:
- Add ADR-0009: Redis-Only Indexing Architecture
- Update ADR-0003 and ADR-0008 for Redis-only approach
- Update website HTML files (index.html, examples.html)
- Update example code (03-with-indexing, 04-versioning)
- Update DATASHEET.md and simple API documentation

BENEFITS:
✅ Simpler architecture - single indexing system
✅ Faster performance - all lookups in-memory Redis
✅ Less code - ~400 lines removed
✅ Cleaner filesystem - no indexes/ directories
✅ Better DX - single sb:"index" tag for everything

MIGRATION:
1. Change sb:"index,unique" to sb:"index" in struct tags
2. Update AutoRegisterIndexes() calls to remove first parameter
3. Update NewCascadeIndexManager() calls to remove indexer parameter
4. Remove WithFileIndexer() from IndexManager initialization

See ADR-0009 for complete details.

### Documentation

* add auto-indexing and cascade delete examples to website ([a41f697](https://github.com/adrianmcphee/smarterbase/commit/a41f697cd219dc6ddc321ba92f16b1106c778937))
* remove before/after comparisons from homepage examples ([4aead46](https://github.com/adrianmcphee/smarterbase/commit/4aead4682714fc91233dc8a56c17f774f4394547))
* remove percentage claims from banners ([3fd0b77](https://github.com/adrianmcphee/smarterbase/commit/3fd0b7726cad59f2e552e8f9a01e7d3d140542b3))
* tone down language in ADR-0008 ([f3ae4b5](https://github.com/adrianmcphee/smarterbase/commit/f3ae4b5c58feddfecc5d6466931ce09ab411dc08))
* update homepage to showcase ADR-0008 features ([43c27a8](https://github.com/adrianmcphee/smarterbase/commit/43c27a8419f2be9f6b6dfe6460215fcc99466cf7))

### Code Refactoring

* remove file-based indexing, Redis-only architecture ([438fd46](https://github.com/adrianmcphee/smarterbase/commit/438fd466ef85564cbf594496f0fe187f26452677))

## [1.11.0](https://github.com/adrianmcphee/smarterbase/compare/v1.10.0...v1.11.0) (2025-11-16)

### Features

* add auto-indexing with struct tags and declarative cascade deletes ([5ed8c9e](https://github.com/adrianmcphee/smarterbase/commit/5ed8c9e8e9111bf04c0918df2201b2fe4d553635))

### Documentation

* add type safety guidance to .ai-context ([90bab48](https://github.com/adrianmcphee/smarterbase/commit/90bab480e947b7649515c1ba43f134cf1b483407))

## [1.10.0](https://github.com/adrianmcphee/smarterbase/compare/v1.9.0...v1.10.0) (2025-10-28)

### Features

* add type-safe migration API with WithTypeSafe helper ([2325311](https://github.com/adrianmcphee/smarterbase/commit/232531153bfe6f4349674ed581be0e9633117fc9))

### Documentation

* add ADR-0006 to index table ([2f53030](https://github.com/adrianmcphee/smarterbase/commit/2f530301366ff6b9c8c7942c96880fdf2dca75b6))
* update ADR-0006 status from Proposed to Accepted ([d9c0356](https://github.com/adrianmcphee/smarterbase/commit/d9c035652ee24dc22668b4d8620727d11554f520))

## [1.9.0](https://github.com/adrianmcphee/smarterbase/compare/v1.8.0...v1.9.0) (2025-10-18)

### Features

* add distributed lock management and counter audit utilities ([c3d37ea](https://github.com/adrianmcphee/smarterbase/commit/c3d37ead2d6a25bfeafe55170afcf266429ab254))

### Documentation

* add production-patterns example to website showcase ([e3d88d6](https://github.com/adrianmcphee/smarterbase/commit/e3d88d6b5b4db93e62e91bf1bd4c721bc038f0fe))
* enhance inline godoc for ADR-0006 helpers and core functions ([a1b7238](https://github.com/adrianmcphee/smarterbase/commit/a1b7238b6b86862c37a42e100971b0d4f45a0d9a))
* update package documentation to include ADR-0006 helpers ([847ec5e](https://github.com/adrianmcphee/smarterbase/commit/847ec5e54c999d378cda1bf9daf0736ddaf09d4a))
* update production-patterns example to use ADR-0006 helpers ([94f0a66](https://github.com/adrianmcphee/smarterbase/commit/94f0a66d2428f39608ab07d8d4eb8fd4b938f7b4))
* update remaining examples to use ADR-0006 QueryWithFallback helper ([ffe54b1](https://github.com/adrianmcphee/smarterbase/commit/ffe54b1287f702813ef142bae7d72ea886012572))
* update website to highlight ADR-0006 QueryWithFallback helper ([67bb8a9](https://github.com/adrianmcphee/smarterbase/commit/67bb8a9a219b9c31004762afd5b9d183a066fc2c))

## [1.8.0](https://github.com/adrianmcphee/smarterbase/compare/v1.7.2...v1.8.0) (2025-10-18)

### Features

* add pragmatic helper functions to reduce boilerplate (ADR-0006) ([46a7379](https://github.com/adrianmcphee/smarterbase/commit/46a73790575513b4413a6fc858fea511a7f730ff))

## [1.7.2](https://github.com/adrianmcphee/smarterbase/compare/v1.7.1...v1.7.2) (2025-10-14)

### Bug Fixes

* run MetricsExporter.Start() in goroutine for test ([46345af](https://github.com/adrianmcphee/smarterbase/commit/46345af0797ce341da64180e382abbc1cb06be72))

## [1.7.1](https://github.com/adrianmcphee/smarterbase/compare/v1.7.0...v1.7.1) (2025-10-14)

### Bug Fixes

* properly handle errors for errcheck linter ([257f291](https://github.com/adrianmcphee/smarterbase/commit/257f2914b786e287f085f56ff046fe94f300793e))

## [1.7.0](https://github.com/adrianmcphee/smarterbase/compare/v1.6.0...v1.7.0) (2025-10-15)

### Features

* **examples**: add production-patterns example demonstrating Redis fallback and query profiling ([de792f9](https://github.com/adrianmcphee/smarterbase/commit/de792f9))
  - Redis-first pattern with automatic fallback to full scans
  - Query profiling and complexity tracking (O(1) vs O(n))
  - Graceful degradation during Redis outages
  - Demonstrates simple key generation per ADR-0005

### Documentation

* **examples**: document two usage patterns - Simple (hectic) and Advanced (tuinplan) ([de792f9](https://github.com/adrianmcphee/smarterbase/commit/de792f9))
* **examples**: update learning paths for simple and advanced patterns ([de792f9](https://github.com/adrianmcphee/smarterbase/commit/de792f9))

### Bug Fixes

* **simple**: fix linting errors in Collection and DB ([de792f9](https://github.com/adrianmcphee/smarterbase/commit/de792f9))
  - Remove unused 'initialized' field
  - Add proper error handling for json operations
  - Fix unchecked Close() errors

## [1.6.0](https://github.com/adrianmcphee/smarterbase/compare/v1.5.0...v1.6.0) (2025-10-14)

### Features

* **helpers**: add RedisOptionsWithOverrides() for mixed explicit/environment config ([#TBD](https://github.com/adrianmcphee/smarterbase/pull/TBD))

### Documentation

* **adr**: add ADR-0005 Core API Helpers Guidance - when and how to use BatchGet[T], KeyBuilder, and RedisOptions ([#TBD](https://github.com/adrianmcphee/smarterbase/pull/TBD))
* **readme**: add Core API Helpers - Best Practices section with usage guidance ([#TBD](https://github.com/adrianmcphee/smarterbase/pull/TBD))
* **examples**: update user-management, ecommerce-orders, and multi-tenant-config to use BatchGet[T] pattern ([#TBD](https://github.com/adrianmcphee/smarterbase/pull/TBD))

### Migration Notes

See [CHANGELOG_DRAFT.md](./CHANGELOG_DRAFT.md) for detailed migration guide from v1.5.0.

## [1.5.0](https://github.com/adrianmcphee/smarterbase/compare/v1.4.0...v1.5.0) (2025-10-14)

### Features

* add Simple API with automatic indexing and versioning support ([307b3da](https://github.com/adrianmcphee/smarterbase/commit/307b3da60686e007b368da15736856446298602c))

### Documentation

* update website quickstart to use RedisOptions helper ([2567cf3](https://github.com/adrianmcphee/smarterbase/commit/2567cf3a4fc56772d333e638f584fbe95332802d))

## [1.4.0](https://github.com/adrianmcphee/smarterbase/compare/v1.3.0...v1.4.0) (2025-10-13)

### Features

* add RedisOptions helper for environment-based configuration ([6e8834e](https://github.com/adrianmcphee/smarterbase/commit/6e8834e85b5d98ed3ae70941e3726f3d69a1fa53))

### Documentation

* add git pull workflow instructions for AI agents in .ai-context ([1462b42](https://github.com/adrianmcphee/smarterbase/commit/1462b42cbc5914eaa039298a567246ed58fd0446))

## [1.3.0](https://github.com/adrianmcphee/smarterbase/compare/v1.2.9...v1.3.0) (2025-10-12)

### Features

* simple, opt-in versioning of stored objects and migrations on read ([6d5fe08](https://github.com/adrianmcphee/smarterbase/commit/6d5fe08f0566d99ced1ed387a14a596fe027ae41))

## [1.2.9](https://github.com/adrianmcphee/smarterbase/compare/v1.2.8...v1.2.9) (2025-10-11)

### Bug Fixes

* update Codecov action to use 'files' instead of deprecated 'file' parameter ([053528a](https://github.com/adrianmcphee/smarterbase/commit/053528a902d586d0665548a135a2cfc9cf91a800))

### Documentation

* remove redundant documentation files ([4f82205](https://github.com/adrianmcphee/smarterbase/commit/4f822051cac419d965b097242eda0d0457409bdb))

## [1.2.8](https://github.com/adrianmcphee/smarterbase/compare/v1.2.7...v1.2.8) (2025-10-11)

### Bug Fixes

* apply aggressive width constraints to prevent mobile overflow ([adc6be9](https://github.com/adrianmcphee/smarterbase/commit/adc6be91983d145ae3a1008afacf848cf238597e))

## [1.2.7](https://github.com/adrianmcphee/smarterbase/compare/v1.2.6...v1.2.7) (2025-10-11)

### Bug Fixes

* prevent horizontal scroll and overflow on mobile with comprehensive CSS fixes ([5e3fa2d](https://github.com/adrianmcphee/smarterbase/commit/5e3fa2dfa6eb401a2cf325df499ff4092ab81e99))

## [1.2.6](https://github.com/adrianmcphee/smarterbase/compare/v1.2.5...v1.2.6) (2025-10-11)

### Bug Fixes

* force container padding with !important to override framework CSS ([82fcefd](https://github.com/adrianmcphee/smarterbase/commit/82fcefd19376bc493d91c636fc5c83e8b771d67b))

## [1.2.5](https://github.com/adrianmcphee/smarterbase/compare/v1.2.4...v1.2.5) (2025-10-11)

### Bug Fixes

* add horizontal padding to containers for mobile responsiveness ([565cb8e](https://github.com/adrianmcphee/smarterbase/commit/565cb8eb44b0472ae6e3c310c671582629c050d5))

## [1.2.4](https://github.com/adrianmcphee/smarterbase/compare/v1.2.3...v1.2.4) (2025-10-11)

### Bug Fixes

* update mobile layout and codecov integration ([341696c](https://github.com/adrianmcphee/smarterbase/commit/341696cb49c8875df64ef46396836dff8adee1d7))

## [1.2.3](https://github.com/adrianmcphee/smarterbase/compare/v1.2.2...v1.2.3) (2025-10-11)

### Bug Fixes

* resolve race condition in TestIntegration_ConcurrentWrites ([e60b751](https://github.com/adrianmcphee/smarterbase/commit/e60b75192862c10f2be8f7e6186244ea5123337e))

## [1.2.2](https://github.com/adrianmcphee/smarterbase/compare/v1.2.1...v1.2.2) (2025-10-11)

### Bug Fixes

* S3 Append should use IsNotFound instead of string matching ([4361f8a](https://github.com/adrianmcphee/smarterbase/commit/4361f8ab29998ff29eba523b9b97b734bb2da062))

### Documentation

* update datasheet to reflect v1.2.0 features ([15b9b74](https://github.com/adrianmcphee/smarterbase/commit/15b9b7496376a8016b98ffaa5ad359a01c1f133b))

## [1.2.1](https://github.com/adrianmcphee/smarterbase/compare/v1.2.0...v1.2.1) (2025-10-11)

### Bug Fixes

* resolve CI test failures and golangci-lint deprecation warnings ([81bf882](https://github.com/adrianmcphee/smarterbase/commit/81bf8821ed0bce52b1bb5e9a91393b7030bf276c))

### Documentation

* highlight self-healing and circuit breaker capabilities ([ae513ee](https://github.com/adrianmcphee/smarterbase/commit/ae513ee854cc0de5aa3c65cf34ee687da0876d24))

## [1.2.0](https://github.com/adrianmcphee/smarterbase/compare/v1.1.3...v1.2.0) (2025-10-11)

### Features

* add circuit breaker to Redis writes and enable auto-repair by default ([d04fffb](https://github.com/adrianmcphee/smarterbase/commit/d04fffb2230aca03580b32c3dcef19e5b7e948ce))

## [1.1.3](https://github.com/adrianmcphee/smarterbase/compare/v1.1.2...v1.1.3) (2025-10-11)

### Bug Fixes

* resolve all golangci-lint errors across codebase ([3f87909](https://github.com/adrianmcphee/smarterbase/commit/3f879098de90506aa03b79091de0bd8e02a0ab01))

## [1.1.2](https://github.com/adrianmcphee/smarterbase/compare/v1.1.1...v1.1.2) (2025-10-11)

### Bug Fixes

* resolve CI test failures and add developer tooling ([5a58686](https://github.com/adrianmcphee/smarterbase/commit/5a5868677fb2e765f15e8db6de08dc3b5f4ce223))

### Documentation

* add GitHub Pages site with dark theme and benefits section ([6883341](https://github.com/adrianmcphee/smarterbase/commit/6883341e6fa27a83ff4b657aba18c24235c2f897))
* enhance examples with compelling value propositions ([7ae5a88](https://github.com/adrianmcphee/smarterbase/commit/7ae5a883d11f3fba51679b3b10ff01bc989cf8dd))
* switch to Font Awesome icons for better reliability ([195316d](https://github.com/adrianmcphee/smarterbase/commit/195316d540715ea23f0f2c133fcbd40c5d854088))

## [1.1.1](https://github.com/adrianmcphee/smarterbase/compare/v1.1.0...v1.1.1) (2025-10-11)

### Bug Fixes

* resolve lint errors and add examples compilation check to CI ([5024884](https://github.com/adrianmcphee/smarterbase/commit/50248844fef5cbabb70019046074ddac02f4de8e))

## [1.1.0](https://github.com/adrianmcphee/smarterbase/compare/v1.0.2...v1.1.0) (2025-10-11)

### Features

* automated versioning and 70% test coverage ([7d7ac1d](https://github.com/adrianmcphee/smarterbase/commit/7d7ac1deda88a281bb2d2317adb272ffc05e663b))
