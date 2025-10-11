# Security Policy

## Supported Versions

We actively support the latest version of SmarterBase with security updates.

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

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

### Disclosure Policy

- We follow responsible disclosure practices
- We will coordinate with you on the disclosure timeline
- Security fixes will be released as soon as possible
- Credit will be given to reporters (unless anonymity is requested)

## Security Best Practices

When using SmarterBase in production:

### 1. **Use S3BackendWithRedisLock for Production**
```go
// ✅ SAFE: Prevents race conditions
backend := smarterbase.NewS3BackendWithRedisLock(s3Client, bucket, redisClient)

// ❌ UNSAFE: Only for single-writer scenarios
backend := smarterbase.NewS3Backend(s3Client, bucket)
```

### 2. **Enable Encryption at Rest**
```go
// Generate or load encryption key from secrets manager
encryptionKey := loadFromSecretsManager() // 32-byte key

// Wrap backend with encryption
encryptedBackend, _ := smarterbase.NewEncryptionBackend(backend, encryptionKey)
```

### 3. **Secure Redis Connection**
```go
redisClient := redis.NewClient(&redis.Options{
    Addr:     "localhost:6379",
    Password: os.Getenv("REDIS_PASSWORD"), // Use env var
    TLS:      &tls.Config{},               // Enable TLS
})
```

### 4. **Use IAM Roles for S3 Access**
```go
// Use IAM roles instead of access keys
cfg, _ := config.LoadDefaultConfig(ctx)
s3Client := s3.NewFromConfig(cfg)
```

### 5. **Set Appropriate Timeouts**
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// All operations respect context timeout
store.GetJSON(ctx, key, &data)
```

### 6. **Validate Input Data**
```go
// Validate before storing
if len(key) == 0 || len(key) > 1024 {
    return errors.New("invalid key length")
}

// Sanitize user input
key = strings.TrimSpace(key)
```

### 7. **Monitor and Alert**
```go
// Enable metrics and logging
logger, _ := smarterbase.NewProductionZapLogger()
metrics := smarterbase.NewPrometheusMetrics(prometheus.DefaultRegisterer)
store := smarterbase.NewStoreWithObservability(backend, logger, metrics)

// Alert on anomalies
// - High error rates
// - Index drift
// - Lock timeouts
```

## Known Security Considerations

### 1. **Redis Security**
- Redis contains indexes but NOT the source data
- If Redis is compromised, rebuild indexes from S3
- Use Redis AUTH and TLS in production
- Run Redis on private network

### 2. **S3 Security**
- S3 bucket should not be public
- Use bucket policies to restrict access
- Enable S3 versioning for data recovery
- Enable S3 access logging

### 3. **Encryption Key Management**
- Store encryption keys in AWS Secrets Manager or HashiCorp Vault
- Rotate keys periodically
- Never commit keys to version control
- Use different keys for different environments

### 4. **Network Security**
- Run application on private subnet
- Use VPC endpoints for S3
- Use security groups to restrict Redis access
- Enable CloudTrail for API auditing

## Dependencies

SmarterBase uses the following security-sensitive dependencies:

- **github.com/aws/aws-sdk-go-v2**: AWS SDK (keep updated)
- **github.com/redis/go-redis/v9**: Redis client (keep updated)
- **crypto/aes**: Standard library cryptography

We monitor these dependencies for security advisories and update promptly.

## Security Updates

Security updates will be announced via:

1. GitHub Security Advisories
2. Release notes in CHANGELOG.md
3. Git tags with version bumps

Subscribe to repository releases to be notified of security updates.

## Questions?

For non-security issues, please open a GitHub issue.

For security concerns, contact the maintainer directly.
