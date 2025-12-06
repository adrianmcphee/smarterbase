# Request for Comments (RFCs)

This directory contains RFCs for significant architectural changes to SmarterBase.

## What is an RFC?

An RFC (Request for Comments) is a design document that describes a major feature or architectural change. RFCs are more comprehensive than ADRs and are used when:
- The change is significant enough to warrant community input
- Multiple approaches need to be explored in depth
- The implementation spans multiple components
- Breaking changes are involved

## Format

Each RFC follows this structure:
- **Title**: Descriptive name
- **Status**: Draft, Under Review, Accepted, Implemented, Rejected
- **Authors**: Who wrote this RFC
- **Created**: Date created
- **Summary**: Brief overview (2-3 sentences)
- **Motivation**: Why this change is needed
- **Detailed Design**: Technical specification
- **Alternatives Considered**: Other approaches evaluated
- **Migration Path**: How to transition from current state
- **Open Questions**: Unresolved issues

## Index

| RFC | Title | Status | Date |
|-----|-------|--------|------|
| [0001](0001-filesystem-native-postgres-protocol.md) | Filesystem-Native Storage with PostgreSQL Wire Protocol | Draft | 2024-12-06 |

## Process

1. **Draft**: Author writes RFC and opens PR
2. **Under Review**: Community provides feedback
3. **Accepted/Rejected**: Maintainers make decision
4. **Implemented**: RFC is realized in code
