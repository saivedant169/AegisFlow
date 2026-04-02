package provider

import (
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/saivedant169/AegisFlow/internal/config"
)

type apiKeyState struct {
	key             string
	weight          int
	useCount        int
	permanentFailed bool
	cooldownUntil   time.Time
}

type keyManager struct {
	mu       sync.Mutex
	keys     []*apiKeyState
	strategy string
	cooldown time.Duration
	next     int
	rng      *rand.Rand
	now      func() time.Time
}

func newKeyManager(singleKey string, apiKeys []config.ProviderAPIKey, strategy string, cooldown time.Duration) *keyManager {
	km := &keyManager{
		strategy: strategy,
		cooldown: cooldown,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
		now:      time.Now,
	}
	if km.cooldown <= 0 {
		km.cooldown = 5 * time.Minute
	}
	for _, apiKey := range apiKeys {
		if apiKey.Key == "" {
			continue
		}
		weight := apiKey.Weight
		if weight <= 0 {
			weight = 1
		}
		km.keys = append(km.keys, &apiKeyState{key: apiKey.Key, weight: weight})
	}
	if len(km.keys) == 0 && singleKey != "" {
		km.keys = append(km.keys, &apiKeyState{key: singleKey, weight: 1})
	}
	return km
}

func (km *keyManager) nextKey() *apiKeyState {
	if km == nil {
		return nil
	}

	km.mu.Lock()
	defer km.mu.Unlock()

	available := km.availableKeysLocked()
	if len(available) == 0 {
		return nil
	}

	var selected *apiKeyState
	switch km.strategy {
	case "random":
		selected = km.pickRandomLocked(available)
	case "least-used":
		selected = km.pickLeastUsedLocked(available)
	default:
		selected = km.pickRoundRobinLocked(available)
	}
	if selected != nil {
		selected.useCount++
	}
	return selected
}

func (km *keyManager) reportStatus(providerName, key string, status int) {
	if km == nil || key == "" {
		return
	}

	km.mu.Lock()
	defer km.mu.Unlock()

	for _, candidate := range km.keys {
		if candidate.key != key {
			continue
		}
		switch status {
		case 401:
			candidate.permanentFailed = true
			log.Printf("provider %s: excluding API key permanently after 401", providerName)
		case 429:
			candidate.cooldownUntil = km.now().Add(km.cooldown)
			log.Printf("provider %s: cooling down API key until %s after 429", providerName, candidate.cooldownUntil.Format(time.RFC3339))
		}
		return
	}
}

func (km *keyManager) healthyCount() int {
	if km == nil {
		return 0
	}
	km.mu.Lock()
	defer km.mu.Unlock()
	return len(km.availableKeysLocked())
}

func (km *keyManager) availableKeysLocked() []*apiKeyState {
	now := km.now()
	available := make([]*apiKeyState, 0, len(km.keys))
	for _, key := range km.keys {
		if key.permanentFailed {
			continue
		}
		if !key.cooldownUntil.IsZero() && key.cooldownUntil.After(now) {
			continue
		}
		available = append(available, key)
	}
	return available
}

func (km *keyManager) weightedKeysLocked(keys []*apiKeyState) []*apiKeyState {
	var weighted []*apiKeyState
	for _, key := range keys {
		for i := 0; i < key.weight; i++ {
			weighted = append(weighted, key)
		}
	}
	return weighted
}

func (km *keyManager) pickRoundRobinLocked(keys []*apiKeyState) *apiKeyState {
	weighted := km.weightedKeysLocked(keys)
	if len(weighted) == 0 {
		return nil
	}
	selected := weighted[km.next%len(weighted)]
	km.next++
	return selected
}

func (km *keyManager) pickRandomLocked(keys []*apiKeyState) *apiKeyState {
	weighted := km.weightedKeysLocked(keys)
	if len(weighted) == 0 {
		return nil
	}
	return weighted[km.rng.Intn(len(weighted))]
}

func (km *keyManager) pickLeastUsedLocked(keys []*apiKeyState) *apiKeyState {
	selected := keys[0]
	for _, key := range keys[1:] {
		if key.useCount < selected.useCount {
			selected = key
		}
	}
	return selected
}
