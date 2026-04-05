package cache

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

type semanticEntry struct {
	tenantID  string
	model     string
	embedding []float64
	response  *types.ChatCompletionResponse
	createdAt time.Time
}

// SemanticCache uses embedding similarity to match near-duplicate requests.
type SemanticCache struct {
	mu        sync.RWMutex
	embedder  Embedder
	threshold float64
	maxSize   int
	entries   []semanticEntry
	hits      int64
	misses    int64
}

func NewSemanticCache(embedder Embedder, threshold float64, maxSize int) *SemanticCache {
	return &SemanticCache{
		embedder:  embedder,
		threshold: threshold,
		maxSize:   maxSize,
		entries:   make([]semanticEntry, 0, maxSize),
	}
}

func (sc *SemanticCache) buildText(req *types.ChatCompletionRequest) string {
	var text string
	for _, m := range req.Messages {
		text += m.Role + ": " + m.Content + "\n"
	}
	return text
}

// GetSemantic searches for a semantically similar cached response.
func (sc *SemanticCache) GetSemantic(tenantID string, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, bool) {
	vec, err := sc.embedder.Embed(context.Background(), sc.buildText(req))
	if err != nil {
		log.Printf("[semantic-cache] embedding failed: %v", err)
		sc.mu.Lock()
		sc.misses++
		sc.mu.Unlock()
		return nil, false
	}

	sc.mu.RLock()
	var bestScore float64
	var bestResp *types.ChatCompletionResponse

	for i := range sc.entries {
		e := &sc.entries[i]
		if e.tenantID != tenantID || e.model != req.Model {
			continue
		}
		score := CosineSimilarity(vec, e.embedding)
		if score > bestScore {
			bestScore = score
			bestResp = e.response
		}
	}
	sc.mu.RUnlock()

	sc.mu.Lock()
	defer sc.mu.Unlock()

	if bestResp != nil && bestScore >= sc.threshold {
		sc.hits++
		return bestResp, true
	}

	sc.misses++
	return nil, false
}

// SetSemantic stores a response with its embedding for semantic matching.
func (sc *SemanticCache) SetSemantic(tenantID string, req *types.ChatCompletionRequest, resp *types.ChatCompletionResponse) {
	vec, err := sc.embedder.Embed(context.Background(), sc.buildText(req))
	if err != nil {
		log.Printf("[semantic-cache] embedding failed on set: %v", err)
		return
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()

	if len(sc.entries) >= sc.maxSize {
		sc.entries = sc.entries[1:] // evict oldest
	}

	sc.entries = append(sc.entries, semanticEntry{
		tenantID:  tenantID,
		model:     req.Model,
		embedding: vec,
		response:  resp,
		createdAt: time.Now(),
	})
}

// Get implements the Cache interface using semantic matching.
// Key is ignored -- matching is done by embedding similarity.
func (sc *SemanticCache) Get(key string) (*types.ChatCompletionResponse, bool) {
	// Semantic cache doesn't support key-based lookup.
	// Use GetSemantic directly for semantic matching.
	return nil, false
}

// Set implements the Cache interface. No-op for semantic cache.
// Use SetSemantic directly.
func (sc *SemanticCache) Set(key string, resp *types.ChatCompletionResponse) {
	// No-op: semantic cache requires the full request for embedding.
}

// Stats returns cache statistics.
func (sc *SemanticCache) Stats() CacheStats {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return CacheStats{
		Hits:    sc.hits,
		Misses:  sc.misses,
		Size:    len(sc.entries),
		MaxSize: sc.maxSize,
		TTL:     "n/a",
	}
}
