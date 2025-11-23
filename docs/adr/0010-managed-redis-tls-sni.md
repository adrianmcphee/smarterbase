# ADR-0010: Automatic TLS and SNI for Managed Redis

**Status:** Accepted
**Date:** 2025-11-23
**Authors:** Adrian McPhee

## Context

When connecting to managed Redis services (DigitalOcean, AWS ElastiCache, Azure Redis), applications must:
1. Enable TLS for secure connections
2. Set the ServerName field for Server Name Indication (SNI) to match the hostname

Current `RedisOptionsWithOverrides` function requires users to manually configure TLS and SNI:

```go
if strings.HasSuffix(redisAddr, ":25061") || os.Getenv("REDIS_TLS_ENABLED") == "true" {
    hostname := strings.Split(redisAddr, ":")[0]
    redisOpts.TLSConfig = &tls.Config{
        ServerName: hostname,
        MinVersion: tls.VersionTLS12,
    }
}
```

This creates friction for users of managed Redis services who expect "batteries-included" configuration helpers.

## Decision

We will enhance `RedisOptions()` and `RedisOptionsWithOverrides()` to automatically:
1. Detect managed Redis addresses (port 25061 for DigitalOcean)
2. Enable TLS automatically for these addresses
3. Extract hostname and set ServerName for proper SNI

## Implementation

```go
// In RedisOptions() and RedisOptionsWithOverrides():
// Automatically enable TLS for known managed Redis ports
tlsEnabled := os.Getenv("REDIS_TLS_ENABLED") == "true" || strings.HasSuffix(addr, ":25061")

if tlsEnabled {
    host := extractHostname(addr)
    opts.TLSConfig = &tls.Config{
        MinVersion: tls.VersionTLS12,
        ServerName: host,
    }
}

// Always ensure ServerName is set when TLS is enabled
if opts.TLSConfig != nil && opts.TLSConfig.ServerName == "" {
    opts.TLSConfig.ServerName = extractHostname(opts.Addr)
}
```

## Consequences

### Positive
- ✅ **Zero configuration** for managed Redis services
- ✅ **Eliminates boilerplate** - no more manual TLS/SNI setup
- ✅ **Works out of the box** with DigitalOcean managed Redis
- ✅ **Maintains backward compatibility** - existing code unchanged
- ✅ **Proper SNI support** - connections work correctly with managed services

### Negative
- ⚠️ **Magic behavior** - TLS enabled automatically based on port detection
- ⚠️ **Limited scope** - only detects DigitalOcean (port 25061) currently
- ⚠️ **Potential false positives** - any service on port 25061 gets TLS enabled

### Neutral
- Extends ADR-0002 (Redis Configuration Ergonomics)
- Environment variable `REDIS_TLS_ENABLED=true` still overrides automatic detection
- Future ports can be added to detection logic
- Users can still disable with `REDIS_TLS_ENABLED=false`

## Alternatives Considered

### Option 1: Add TLS Parameter to RedisOptionsWithOverrides
```go
func RedisOptionsWithOverrides(addr, password string, poolSize, minIdleConns, tlsEnabled int) *redis.Options
```

**Rejected:** Breaking API change, doesn't solve the "what should the default be?" problem.

### Option 2: Environment Variable Only
Keep current behavior, document that users should set `REDIS_TLS_ENABLED=true` for managed Redis.

**Rejected:** Still requires manual configuration, defeats purpose of ergonomic helpers.

### Option 3: No Automatic Detection
Keep current explicit behavior.

**Rejected:** Doesn't solve user pain with managed Redis services.

## Future Considerations

Could extend automatic detection to other managed Redis ports:
- AWS ElastiCache: 6379/6380 (TLS optional)
- Azure Redis: 6380 (TLS required)
- GCP Memorystore: 6379 (TLS required)

But starting with DigitalOcean (most common complaint) is appropriate.
