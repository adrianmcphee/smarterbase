# Security Policy

## Supported Versions

We actively support the latest version of SmarterBase with security updates.

| Version | Supported          |
| ------- | ------------------ |
| 3.x.x   | :white_check_mark: |
| < 3.0   | :x:                |

## Reporting a Vulnerability

If you discover a security vulnerability in SmarterBase, please report it by emailing the maintainer directly rather than creating a public issue.

**Please DO NOT create a public GitHub issue for security vulnerabilities.**

### What to Include

When reporting a vulnerability, please include:

1. **Description** of the vulnerability
2. **Steps to reproduce** the issue
3. **Potential impact** of the vulnerability
4. **Suggested fix** (if you have one)
5. **Your contact information** for follow-up

### Response Timeline

- **Initial Response**: Within 48 hours
- **Status Update**: Within 7 days
- **Fix Timeline**: Depends on severity
  - Critical: 1-7 days
  - High: 7-14 days
  - Medium: 14-30 days
  - Low: 30+ days or next release

## Security Considerations

### 1. Development Database Only

SmarterBase is designed for **development and prototyping**, not production workloads. Security considerations:

- No authentication by default (configure `password` if needed)
- No encryption at rest
- No TLS support currently
- Single-server only

For production, migrate to PostgreSQL.

### 2. File System Security

SmarterBase stores data as JSON files:

```
./data/
├── _schema/
│   └── users.json      # Schema definition
└── users.jsonl         # All user data
```

Ensure appropriate file permissions:
```bash
chmod 700 ./data
```

### 3. Network Security

By default, SmarterBase listens on localhost. For remote access:
- Use a reverse proxy with TLS
- Restrict access via firewall
- Use password authentication

### 4. Data Visibility

All data is stored as human-readable JSON. This is a feature for development but means:
- Sensitive data is visible in plain text
- Do not store secrets, passwords, or PII
- Use real PostgreSQL for sensitive data

## Questions?

For non-security issues, please open a GitHub issue.

For security concerns, contact the maintainer directly.
