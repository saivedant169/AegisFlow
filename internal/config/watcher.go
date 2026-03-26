package config

import (
	"log"
	"os"
	"sync"
	"time"
)

// Watcher monitors a config file for changes and reloads it.
type Watcher struct {
	path     string
	mu       sync.RWMutex
	cfg      *Config
	lastMod  time.Time
	onChange func(*Config)
	stop     chan struct{}
}

func NewWatcher(path string, cfg *Config, onChange func(*Config)) *Watcher {
	info, _ := os.Stat(path)
	var lastMod time.Time
	if info != nil {
		lastMod = info.ModTime()
	}
	return &Watcher{
		path:     path,
		cfg:      cfg,
		lastMod:  lastMod,
		onChange: onChange,
		stop:     make(chan struct{}),
	}
}

func (w *Watcher) Start(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				w.check()
			case <-w.stop:
				return
			}
		}
	}()
}

func (w *Watcher) Stop() {
	close(w.stop)
}

func (w *Watcher) GetConfig() *Config {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cfg
}

func (w *Watcher) check() {
	info, err := os.Stat(w.path)
	if err != nil {
		return
	}

	if !info.ModTime().After(w.lastMod) {
		return
	}

	newCfg, err := Load(w.path)
	if err != nil {
		log.Printf("config reload failed: %v", err)
		return
	}

	w.mu.Lock()
	w.cfg = newCfg
	w.lastMod = info.ModTime()
	w.mu.Unlock()

	log.Printf("config reloaded from %s", w.path)

	if w.onChange != nil {
		w.onChange(newCfg)
	}
}
