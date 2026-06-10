package evidence

import (
	"sync"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// Recorder records an action into an evidence chain. Both a single SessionChain
// and a ChainRegistry satisfy it, so callers (e.g. the MCP gateway) don't care
// whether records go to one chain or are split per session.
type Recorder interface {
	Record(env *envelope.ActionEnvelope) (*Record, error)
}

// ChainRegistry keeps one evidence chain per session instead of funneling every
// action through a single global chain. That removes the single write-mutex
// bottleneck (different sessions append in parallel) and bounds memory: idle
// sessions are evicted, so usage is O(active sessions x window) rather than
// O(everything since boot).
type ChainRegistry struct {
	mu      sync.Mutex
	chains  map[string]*registryEntry
	key     []byte
	maxIdle time.Duration
	stop    chan struct{}
}

type registryEntry struct {
	chain    *SessionChain
	lastUsed time.Time
}

const defaultSessionFallback = "default"

// NewChainRegistry creates a registry whose chains are signed with key (may be
// nil for unsigned). It starts a janitor that evicts sessions idle longer than
// maxIdle; pass 0 for a sensible default.
func NewChainRegistry(key []byte) *ChainRegistry {
	r := &ChainRegistry{
		chains:  make(map[string]*registryEntry),
		key:     key,
		maxIdle: time.Hour,
		stop:    make(chan struct{}),
	}
	go r.janitor()
	return r
}

func (r *ChainRegistry) janitor() {
	t := time.NewTicker(r.maxIdle / 4)
	defer t.Stop()
	for {
		select {
		case <-r.stop:
			return
		case now := <-t.C:
			r.mu.Lock()
			for id, e := range r.chains {
				if now.Sub(e.lastUsed) > r.maxIdle {
					delete(r.chains, id)
				}
			}
			r.mu.Unlock()
		}
	}
}

// chainFor returns the chain for a session, creating a signed one on first use.
func (r *ChainRegistry) chainFor(sessionID string) *SessionChain {
	if sessionID == "" {
		sessionID = defaultSessionFallback
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.chains[sessionID]
	if !ok {
		e = &registryEntry{chain: NewSignedSessionChain(sessionID, r.key)}
		r.chains[sessionID] = e
	}
	e.lastUsed = time.Now()
	return e.chain
}

// Record routes the action to its session's chain.
func (r *ChainRegistry) Record(env *envelope.ActionEnvelope) (*Record, error) {
	return r.chainFor(env.Actor.SessionID).Record(env)
}

// get returns the chain for a session without creating one.
func (r *ChainRegistry) get(sessionID string) *SessionChain {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.chains[sessionID]; ok {
		return e.chain
	}
	return nil
}

// chains snapshot for listing.
func (r *ChainRegistry) all() []*SessionChain {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*SessionChain, 0, len(r.chains))
	for _, e := range r.chains {
		out = append(out, e.chain)
	}
	return out
}

// Key returns the signing key (used by callers that need to verify signatures).
func (r *ChainRegistry) Key() []byte { return r.key }

// Close stops the eviction janitor.
func (r *ChainRegistry) Close() {
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
}
