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
