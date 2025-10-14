package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/adrianmcphee/smarterbase"
	"github.com/redis/go-redis/v9"
)

// Article represents a content entity with Redis-backed indexing and fallback patterns
type Article struct {
	ID         string    `json:"id"`
	AuthorID   string    `json:"author_id"`
	CategoryID string    `json:"category_id"`
	Title      string    `json:"title"`
	Body       string    `json:"body"`
	Status     string    `json:"status"` // draft, published, archived
	ViewCount  int       `json:"view_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ArticleStore demonstrates production-ready patterns:
// - Redis indexing with automatic fallback to full scans
// - Query profiling and complexity tracking
// - Observability for index usage and performance
type ArticleStore struct {
	base         *smarterbase.Store
	indexManager *smarterbase.IndexManager
	redisIndexer *smarterbase.RedisIndexer
}

// NewArticleStore creates a production-ready store with resilience patterns
func NewArticleStore(backend smarterbase.Backend, redisClient *redis.Client) *ArticleStore {
	base := smarterbase.NewStoreWithObservability(
		backend,
		&smarterbase.StdLogger{},
		smarterbase.NewInMemoryMetrics(),
	)

	// Create Redis indexer with multi-value indexes
	var redisIndexer *smarterbase.RedisIndexer
	if redisClient != nil {
		redisIndexer = smarterbase.NewRedisIndexer(redisClient)

		// Register indexes for fast O(1) lookups
		redisIndexer.RegisterMultiIndex(&smarterbase.MultiIndexSpec{
			Name:        "articles-by-author",
			EntityType:  "articles",
			ExtractFunc: smarterbase.ExtractJSONField("author_id"),
		})

		redisIndexer.RegisterMultiIndex(&smarterbase.MultiIndexSpec{
			Name:        "articles-by-category",
			EntityType:  "articles",
			ExtractFunc: smarterbase.ExtractJSONField("category_id"),
		})

		redisIndexer.RegisterMultiIndex(&smarterbase.MultiIndexSpec{
			Name:        "articles-by-status",
			EntityType:  "articles",
			ExtractFunc: smarterbase.ExtractJSONField("status"),
		})
	}

	// Create index manager
	indexManager := smarterbase.NewIndexManager(base)
	if redisIndexer != nil {
		indexManager = indexManager.WithRedisIndexer(redisIndexer)
	}

	return &ArticleStore{
		base:         base,
		indexManager: indexManager,
		redisIndexer: redisIndexer,
	}
}

// Helper for consistent key generation (ADR-0005: use fmt.Sprintf for simple keys)
func (s *ArticleStore) articleKey(id string) string {
	return fmt.Sprintf("articles/%s.json", id)
}

// CreateArticle creates a new article with automatic index updates
func (s *ArticleStore) CreateArticle(ctx context.Context, article *Article) error {
	if article.ID == "" {
		article.ID = smarterbase.NewID()
	}
	article.CreatedAt = smarterbase.Now()
	article.UpdatedAt = smarterbase.Now()

	return s.indexManager.Create(ctx, s.articleKey(article.ID), article)
}

// GetArticle retrieves an article by ID
func (s *ArticleStore) GetArticle(ctx context.Context, articleID string) (*Article, error) {
	var article Article
	if err := s.base.GetJSON(ctx, s.articleKey(articleID), &article); err != nil {
		return nil, err
	}
	return &article, nil
}

// ListAuthorArticles demonstrates the Redis fallback pattern:
// 1. Try Redis index first (O(1) lookup)
// 2. Fall back to full scan if Redis is unavailable
// 3. Track which path was used for observability
func (s *ArticleStore) ListAuthorArticles(ctx context.Context, authorID string) ([]*Article, error) {
	// Start profiling
	profiler := smarterbase.GetProfilerFromContext(ctx)
	profile := profiler.StartProfile("ListAuthorArticles")
	if profile != nil {
		profile.FilterFields = []string{"author_id"}
		defer func() {
			profiler.Record(profile)
		}()
	}

	// Try Redis index first (O(1) lookup)
	if s.redisIndexer != nil {
		keys, err := s.redisIndexer.Query(ctx, "articles", "author_id", authorID)
		if err == nil {
			// Use BatchGet[T] for type-safe, efficient loading
			articles, err := smarterbase.BatchGet[Article](ctx, s.base, keys)

			// Record successful index path
			if profile != nil {
				profile.Complexity = smarterbase.ComplexityO1
				profile.IndexUsed = "redis:articles-by-author"
				profile.StorageOps = len(keys)
				profile.ResultCount = len(articles)
			}

			return articles, err
		}

		// Redis failed - log but don't fail the request
		log.Printf("Redis index failed, falling back to full scan: %v", err)
	}

	// Fallback to full scan (O(n) query)
	var articles []*Article
	err := s.base.Query("articles/").
		FilterJSON(func(obj map[string]interface{}) bool {
			aid, _ := obj["author_id"].(string)
			return aid == authorID
		}).
		All(ctx, &articles)

	// Record fallback path
	if profile != nil {
		profile.Complexity = smarterbase.ComplexityON
		profile.IndexUsed = "none:full-scan"
		profile.FallbackPath = true
		profile.ResultCount = len(articles)
		profile.Error = err
	}

	return articles, err
}

// ListCategoryArticles demonstrates profiling with sorting
func (s *ArticleStore) ListCategoryArticles(ctx context.Context, categoryID string, limit int) ([]*Article, error) {
	// Start profiling
	profiler := smarterbase.GetProfilerFromContext(ctx)
	profile := profiler.StartProfile("ListCategoryArticles")
	if profile != nil {
		profile.FilterFields = []string{"category_id"}
		defer func() {
			profiler.Record(profile)
		}()
	}

	// Try Redis index first
	if s.redisIndexer != nil {
		keys, err := s.redisIndexer.Query(ctx, "articles", "category_id", categoryID)
		if err == nil {
			articles, err := smarterbase.BatchGet[Article](ctx, s.base, keys)
			if err == nil {
				// Sort by view count (most viewed first)
				// Note: This requires loading all articles, but the index makes that fast
				sortByViewCount(articles)

				// Apply limit
				if limit > 0 && len(articles) > limit {
					articles = articles[:limit]
				}

				if profile != nil {
					profile.Complexity = smarterbase.ComplexityO1
					profile.IndexUsed = "redis:articles-by-category"
					profile.StorageOps = len(keys)
					profile.ResultCount = len(articles)
				}

				return articles, nil
			}
		}
	}

	// Fallback to full scan with sorting
	var articles []*Article
	err := s.base.Query("articles/").
		FilterJSON(func(obj map[string]interface{}) bool {
			cid, _ := obj["category_id"].(string)
			return cid == categoryID
		}).
		Limit(limit).
		All(ctx, &articles)

	if profile != nil {
		profile.Complexity = smarterbase.ComplexityON
		profile.IndexUsed = "none:full-scan"
		profile.FallbackPath = true
		profile.ResultCount = len(articles)
		profile.Error = err
	}

	return articles, err
}

// GetPublishedArticles demonstrates counting with fallback
func (s *ArticleStore) GetPublishedArticlesCount(ctx context.Context) (int, error) {
	// Start profiling
	profiler := smarterbase.GetProfilerFromContext(ctx)
	profile := profiler.StartProfile("GetPublishedArticlesCount")
	defer func() {
		if profile != nil {
			profiler.Record(profile)
		}
	}()

	// Try Redis index first (O(1) count)
	if s.redisIndexer != nil {
		count, err := s.redisIndexer.Count(ctx, "articles", "status", "published")
		if err == nil {
			if profile != nil {
				profile.Complexity = smarterbase.ComplexityO1
				profile.IndexUsed = "redis:articles-by-status"
				profile.StorageOps = 1
				profile.ResultCount = int(count)
			}
			return int(count), nil
		}
	}

	// Fallback to full scan and count
	count, err := s.base.Query("articles/").
		FilterJSON(func(obj map[string]interface{}) bool {
			status, _ := obj["status"].(string)
			return status == "published"
		}).
		Count(ctx)

	if profile != nil {
		profile.Complexity = smarterbase.ComplexityON
		profile.IndexUsed = "none:full-scan"
		profile.FallbackPath = true
		profile.ResultCount = count
		profile.Error = err
	}

	return count, err
}

// UpdateArticle updates an article with automatic index coordination
func (s *ArticleStore) UpdateArticle(ctx context.Context, article *Article) error {
	article.UpdatedAt = smarterbase.Now()
	return s.indexManager.Update(ctx, s.articleKey(article.ID), article)
}

// DeleteArticle deletes an article with automatic index cleanup
func (s *ArticleStore) DeleteArticle(ctx context.Context, articleID string) error {
	return s.indexManager.Delete(ctx, s.articleKey(articleID))
}

// Helper to sort articles by view count
func sortByViewCount(articles []*Article) {
	// Simple bubble sort for demo purposes
	for i := 0; i < len(articles); i++ {
		for j := i + 1; j < len(articles); j++ {
			if articles[i].ViewCount < articles[j].ViewCount {
				articles[i], articles[j] = articles[j], articles[i]
			}
		}
	}
}

func main() {
	ctx := context.Background()

	fmt.Println("\n=== Production Patterns with SmarterBase ===")
	fmt.Println("\n📋 THE CHALLENGE:")
	fmt.Println("Production systems need:")
	fmt.Println("  • Fast queries (O(1) indexed lookups)")
	fmt.Println("  • Resilience (work even when Redis is down)")
	fmt.Println("  • Observability (know which code paths are used)")
	fmt.Println("  • Performance tracking (identify slow queries)")
	fmt.Println("\n✨ THE SMARTERBASE SOLUTION:")
	fmt.Println("  ✅ Redis-first pattern - O(1) lookups when Redis is available")
	fmt.Println("  ✅ Automatic fallback - Seamlessly falls back to O(n) scan")
	fmt.Println("  ✅ Query profiling - Track complexity and index usage")
	fmt.Println("  ✅ Zero downtime - Application keeps working during Redis outages")
	fmt.Println()

	// Development setup
	backend := smarterbase.NewFilesystemBackend("./data")
	defer backend.Close()

	// Redis configuration from environment (REDIS_ADDR, REDIS_PASSWORD, REDIS_DB)
	// Defaults to localhost:6379 for local development
	redisClient := redis.NewClient(smarterbase.RedisOptions())
	defer redisClient.Close()

	// Note: If Redis is unavailable, store still works with full scans
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Printf("⚠️  Redis unavailable (will use fallback scans): %v", err)
	} else {
		log.Println("✅ Redis connected (will use indexed lookups)")
	}

	// Create store with profiling enabled
	ctx = smarterbase.WithProfiler(ctx, smarterbase.NewQueryProfiler())
	store := NewArticleStore(backend, redisClient)

	fmt.Println("\n=== Running Example Operations ===")

	// 1. Create articles
	fmt.Println("\n1. Creating articles...")
	articles := []*Article{
		{
			AuthorID:   "author-1",
			CategoryID: "tech",
			Title:      "Introduction to SmarterBase",
			Body:       "SmarterBase is a production-ready storage layer...",
			Status:     "published",
			ViewCount:  1500,
		},
		{
			AuthorID:   "author-1",
			CategoryID: "tech",
			Title:      "Advanced Indexing Patterns",
			Body:       "Learn how to build resilient systems...",
			Status:     "published",
			ViewCount:  800,
		},
		{
			AuthorID:   "author-2",
			CategoryID: "business",
			Title:      "Building a SaaS Platform",
			Body:       "Key considerations for SaaS architecture...",
			Status:     "published",
			ViewCount:  2200,
		},
		{
			AuthorID:   "author-1",
			CategoryID: "tech",
			Title:      "Draft: Future Features",
			Body:       "Coming soon...",
			Status:     "draft",
			ViewCount:  0,
		},
	}

	for _, article := range articles {
		if err := store.CreateArticle(ctx, article); err != nil {
			log.Fatalf("Failed to create article: %v", err)
		}
		fmt.Printf("   Created: %s (status: %s, views: %d)\n", article.Title, article.Status, article.ViewCount)
	}

	// 2. List articles by author (demonstrates Redis → fallback pattern)
	fmt.Println("\n2. Listing articles by author-1...")
	authorArticles, err := store.ListAuthorArticles(ctx, "author-1")
	if err != nil {
		log.Fatalf("Failed to list articles: %v", err)
	}
	fmt.Printf("   Found %d articles\n", len(authorArticles))
	for _, a := range authorArticles {
		fmt.Printf("   - %s (%s)\n", a.Title, a.Status)
	}

	// 3. List articles by category with limit (demonstrates sorting with fallback)
	fmt.Println("\n3. Listing top 2 tech articles...")
	techArticles, err := store.ListCategoryArticles(ctx, "tech", 2)
	if err != nil {
		log.Fatalf("Failed to list category articles: %v", err)
	}
	fmt.Printf("   Found %d tech articles\n", len(techArticles))
	for _, a := range techArticles {
		fmt.Printf("   - %s (views: %d)\n", a.Title, a.ViewCount)
	}

	// 4. Count published articles (demonstrates counting with fallback)
	fmt.Println("\n4. Counting published articles...")
	count, err := store.GetPublishedArticlesCount(ctx)
	if err != nil {
		log.Fatalf("Failed to count articles: %v", err)
	}
	fmt.Printf("   Total published: %d articles\n", count)

	// 5. Update article
	fmt.Println("\n5. Updating article view count...")
	authorArticles[0].ViewCount += 100
	if err := store.UpdateArticle(ctx, authorArticles[0]); err != nil {
		log.Fatalf("Failed to update article: %v", err)
	}
	fmt.Printf("   Updated: %s (views: %d)\n", authorArticles[0].Title, authorArticles[0].ViewCount)

	// 6. Show profiling results
	fmt.Println("\n6. Query Profiling Results:")
	profiler := smarterbase.GetProfilerFromContext(ctx)
	profiles := profiler.GetProfiles()

	if len(profiles) > 0 {
		fmt.Println("\n   Query Performance:")
		for _, p := range profiles {
			fallbackIndicator := ""
			if p.FallbackPath {
				fallbackIndicator = " [FALLBACK]"
			}
			fmt.Printf("   • %s: %s%s\n", p.Method, p.Complexity, fallbackIndicator)
			fmt.Printf("     - Index: %s\n", p.IndexUsed)
			fmt.Printf("     - Storage Ops: %d\n", p.StorageOps)
			fmt.Printf("     - Results: %d\n", p.ResultCount)
			fmt.Printf("     - Duration: %v\n", p.Duration)
		}

		fmt.Println("\n   💡 KEY INSIGHTS:")
		fmt.Println("   • O(1) queries use Redis indexes (fast)")
		fmt.Println("   • O(n) queries scan all records (slower, but reliable)")
		fmt.Println("   • [FALLBACK] markers show Redis was unavailable")
		fmt.Println("   • StorageOps shows how many S3/filesystem operations occurred")
	} else {
		fmt.Println("   No profiling data collected (profiler may not be enabled)")
	}

	fmt.Println("\n=== Example Complete ===")
	fmt.Println("\n🎯 PRODUCTION BEST PRACTICES DEMONSTRATED:")
	fmt.Println("  1. Always provide fallback paths (Redis → Full scan)")
	fmt.Println("  2. Use query profiling to identify slow paths")
	fmt.Println("  3. Track complexity (O(1) vs O(n)) for optimization")
	fmt.Println("  4. Monitor fallback usage to detect Redis issues")
	fmt.Println("  5. Use BatchGet[T] for type-safe bulk loading")
}
