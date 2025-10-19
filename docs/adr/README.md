# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records (ADRs) for SmarterBase.

## What is an ADR?

An Architecture Decision Record (ADR) captures an important architectural decision made along with its context and consequences. ADRs help teams understand:
- **Why** a decision was made
- **What alternatives** were considered
- **What consequences** resulted from the decision

## Format

Each ADR follows this structure:
- **Title**: Short descriptive name
- **Status**: Proposed, Accepted, Deprecated, Superseded
- **Context**: The issue motivating this decision
- **Decision**: The change we're proposing or have agreed to
- **Consequences**: The results of applying this decision (positive, negative, neutral)

## Index

| ADR | Title | Status | Date |
|-----|-------|--------|------|
| [0001](0001-schema-versioning-and-migrations.md) | Schema Versioning and Migrations | Accepted | 2025-10-12 |
| [0002](0002-redis-configuration-ergonomics.md) | Redis Configuration Ergonomics | Accepted | 2025-10-13 |
| [0003](0003-simple-api-layer.md) | Simple API Layer for Improved Developer Experience | Accepted | 2025-10-14 |
| [0004](0004-simple-api-versioning.md) | Simple API Versioning Discoverability | Accepted | 2025-10-14 |
| [0005](0005-core-api-helpers-guidance.md) | Core API Helpers - When and How to Use | Accepted | 2025-10-14 |
| [0006](0006-collection-api.md) | Pragmatic Helper Functions to Reduce Boilerplate | Accepted | 2025-01-18 |

## References

- [ADR documentation](https://adr.github.io/)
- [Why ADRs matter](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions)
