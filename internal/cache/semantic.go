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

	storeCh   chan storeJob
	done      chan struct{}
	closeOnce sync.Once
}

// storeJob is a deferred insert handled off the request's critical path. text
// is prebuilt so the worker never needs the request, and embedding carries the
// vector already computed during lookup so the worker skips a second embed call.
type storeJob struct {
	tenantID  string
	model     string
	text      string
	embedding []float64
	resp      *types.ChatCompletionResponse
}

func NewSemanticCache(embedder Embedder, threshold float64, maxSize int) *SemanticCache {
	sc := &SemanticCache{
		embedder:  embedder,
		threshold: threshold,
		maxSize:   maxSize,
		entries:   make([]semanticEntry, 0, maxSize),
		storeCh:   make(chan storeJob, 256),
		done:      make(chan struct{}),
	}
	go sc.storeWorker()
	return sc
}

// storeWorker drains queued inserts so embedding-on-store and the insert itself
// stay off the request path.
func (sc *SemanticCache) storeWorker() {
	for {
		select {
		case <-sc.done:
			return
		case job := <-sc.storeCh:
			vec := job.embedding
			if vec == nil {
				v, err := sc.embedder.Embed(context.Background(), job.text)
				if err != nil {
					log.Printf("[semantic-cache] embedding failed on async store: %v", err)
					continue
				}
				vec = v
			}
			sc.insert(job.tenantID, job.model, vec, job.resp)
		}
	}
}

// Close stops the background store worker.
func (sc *SemanticCache) Close() {
	sc.closeOnce.Do(func() { close(sc.done) })
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
	resp, _, ok := sc.GetSemanticWithEmbedding(tenantID, req)
	return resp, ok
}

// GetSemanticWithEmbedding is GetSemantic but also returns the embedding it
// computed for the request. On a miss the caller can hand that vector to
// StoreAsync so the store doesn't embed the same text a second time — turning
// the previous two embedding round-trips per miss into one.
func (sc *SemanticCache) GetSemanticWithEmbedding(tenantID string, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, []float64, bool) {
	vec, err := sc.embedder.Embed(context.Background(), sc.buildText(req))
	if err != nil {
		log.Printf("[semantic-cache] embedding failed: %v", err)
		sc.mu.Lock()
		sc.misses++
		sc.mu.Unlock()
		return nil, nil, false
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
		return bestResp, vec, true
	}

	sc.misses++
	return nil, vec, false
}

// SetSemantic stores a response, embedding the request synchronously. Kept for
// the Cache contract and callers that want a blocking insert; the hot request
// path uses StoreAsync with a reused embedding instead.
func (sc *SemanticCache) SetSemantic(tenantID string, req *types.ChatCompletionRequest, resp *types.ChatCompletionResponse) {
	vec, err := sc.embedder.Embed(context.Background(), sc.buildText(req))
	if err != nil {
		log.Printf("[semantic-cache] embedding failed on set: %v", err)
		return
	}
	sc.insert(tenantID, req.Model, vec, resp)
}

// StoreAsync queues a response for insertion off the request's critical path.
// embedding is the vector already computed by GetSemanticWithEmbedding; pass nil
// to have the worker embed lazily. If the queue is full the insert is dropped
// (the cache is best-effort).
func (sc *SemanticCache) StoreAsync(tenantID string, req *types.ChatCompletionRequest, resp *types.ChatCompletionResponse, embedding []float64) {
	job := storeJob{
		tenantID:  tenantID,
		model:     req.Model,
		text:      sc.buildText(req),
		embedding: embedding,
		resp:      resp,
	}
	select {
	case <-sc.done:
	case sc.storeCh <- job:
	default:
		log.Printf("[semantic-cache] store queue full — dropping insert")
	}
}

// insert appends an entry under lock, evicting the oldest past maxSize.
func (sc *SemanticCache) insert(tenantID, model string, vec []float64, resp *types.ChatCompletionResponse) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if len(sc.entries) >= sc.maxSize {
		sc.entries = sc.entries[1:] // evict oldest
	}

	sc.entries = append(sc.entries, semanticEntry{
		tenantID:  tenantID,
		model:     model,
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
