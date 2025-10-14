# Production Patterns Example

This example demonstrates production-ready patterns for building resilient systems with SmarterBase:

## Key Patterns

### 1. Redis Fallback Pattern
```go
// Try Redis index first (O(1) lookup)
if s.redisIndexer != nil {
    keys, err := s.redisIndexer.Query(ctx, "articles", "author_id", authorID)
    if err == nil {
        return smarterbase.BatchGet[Article](ctx, s.base, keys)
    }
}

// Fallback to full scan (O(n) query)
var articles []*Article
err := s.base.Query("articles/").
    FilterJSON(func(obj map[string]interface{}) bool {
        aid, _ := obj["author_id"].(string)
        return aid == authorID
    }).
    All(ctx, &articles)
```

**Why this matters:**
- ✅ Fast queries when Redis is available (O(1))
- ✅ System keeps working when Redis is down (O(n) fallback)
- ✅ No user-facing errors during Redis outages
- ✅ Graceful degradation instead of hard failures

### 2. Query Profiling & Complexity Tracking
```go
// Start profiling
profiler := smarterbase.GetProfilerFromContext(ctx)
profile := profiler.StartProfile("ListAuthorArticles")
if profile != nil {
    profile.FilterFields = []string{"author_id"}
    defer func() {
        profiler.Record(profile)
    }()
}

// ... execute query ...

// Record metrics
if profile != nil {
    profile.Complexity = smarterbase.ComplexityO1  // or ComplexityON
    profile.IndexUsed = "redis:articles-by-author"
    profile.StorageOps = len(keys)
    profile.ResultCount = len(articles)
    profile.FallbackPath = true  // if fallback was used
}
```

**Why this matters:**
- ✅ Know which queries are slow (O(n) vs O(1))
- ✅ Identify when fallback paths are triggered
- ✅ Track storage operations for cost optimization
- ✅ Make data-driven decisions about indexing strategy

### 3. Simple Key Generation (ADR-0005)
```go
// ✅ CORRECT: Use fmt.Sprintf for simple keys
func (s *ArticleStore) articleKey(id string) string {
    return fmt.Sprintf("articles/%s.json", id)
}

// ❌ AVOID: KeyBuilder is overkill for simple keys
// propertyKB := smarterbase.KeyBuilder{Prefix: "articles", Suffix: ".json"}
// return propertyKB.Key(id)
```

**Why this matters:**
- ✅ Clearer and more maintainable
- ✅ No indirection for simple keys
- ✅ Reserve KeyBuilder for truly complex multi-segment keys

## When to Use These Patterns

### Use Redis Fallback When:
- ✅ You need fast queries (O(1) indexed lookups)
- ✅ You can't tolerate downtime during Redis outages
- ✅ Your data can be queried via full scan as backup
- ✅ You want graceful degradation

### Use Query Profiling When:
- ✅ You're optimizing query performance
- ✅ You need to justify adding indexes (show O(n) → O(1) improvement)
- ✅ You want to monitor fallback path usage
- ✅ You're tracking storage operation costs

### DON'T Use These Patterns When:
- ❌ Your queries are already fast enough (<100ms)
- ❌ You only have a few hundred records (full scan is fine)
- ❌ You need sub-10ms latency (consider in-memory cache)
- ❌ Your data is write-heavy (indexing overhead may hurt)

## Running the Example

```bash
# Start Redis (optional - example works without it)
docker run -d -p 6379:6379 redis:7-alpine

# Run example
cd examples/production-patterns
go run main.go
```

## Sample Output

```
=== Production Patterns with SmarterBase ===

📋 THE CHALLENGE:
Production systems need:
  • Fast queries (O(1) indexed lookups)
  • Resilience (work even when Redis is down)
  • Observability (know which code paths are used)
  • Performance tracking (identify slow queries)

✨ THE SMARTERBASE SOLUTION:
  ✅ Redis-first pattern - O(1) lookups when Redis is available
  ✅ Automatic fallback - Seamlessly falls back to O(n) scan
  ✅ Query profiling - Track complexity and index usage
  ✅ Zero downtime - Application keeps working during Redis outages

...

6. Query Profiling Results:

   Query Performance:
   • ListAuthorArticles: O(1)
     - Index: redis:articles-by-author
     - Storage Ops: 3
     - Results: 3
     - Duration: 12ms
   • ListCategoryArticles: O(1)
     - Index: redis:articles-by-category
     - Storage Ops: 2
     - Results: 2
     - Duration: 8ms
   • GetPublishedArticlesCount: O(1)
     - Index: redis:articles-by-status
     - Storage Ops: 1
     - Results: 3
     - Duration: 4ms

   💡 KEY INSIGHTS:
   • O(1) queries use Redis indexes (fast)
   • O(n) queries scan all records (slower, but reliable)
   • [FALLBACK] markers show Redis was unavailable
   • StorageOps shows how many S3/filesystem operations occurred
```

## Architecture Benefits

1. **Resilience**: Application never goes down due to Redis outages
2. **Performance**: O(1) lookups when Redis is healthy
3. **Observability**: Know exactly which queries are slow
4. **Cost Optimization**: Track storage operations to optimize S3 costs
5. **Gradual Degradation**: Slower queries are better than no queries

## Related Examples

- **[ecommerce-orders](../ecommerce-orders)** - Redis indexing without fallback (assumes Redis is always available)
- **[user-management](../user-management)** - Basic IndexManager usage
- **[simple/03-with-indexing](../simple/03-with-indexing)** - Introduction to indexing

## Further Reading

- [ADR-0005: Core API Helpers Guidance](../../docs/adr/0005-core-api-helpers-guidance.md)
- [Architecture Documentation](../../ARCHITECTURE.md)
- [Query Performance Guide](../../docs/PERFORMANCE.md)
