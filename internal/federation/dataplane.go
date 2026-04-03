package federation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// DataPlane polls a control plane for config and pushes status/metrics.
type DataPlane struct {
	name         string
	controlURL   string
	token        string
	syncInterval time.Duration
	stopCh       chan struct{}
	client       *http.Client
}

// NewDataPlane creates a data plane that syncs with the given control plane URL.
func NewDataPlane(name, controlURL, token string, syncInterval time.Duration) *DataPlane {
	return &DataPlane{
		name:         name,
		controlURL:   controlURL,
		token:        token,
		syncInterval: syncInterval,
		stopCh:       make(chan struct{}),
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

// Start begins the background sync loop.
func (dp *DataPlane) Start() {
	go dp.syncLoop()
	log.Printf("federation data plane started (name: %s, control: %s, interval: %s)", dp.name, dp.controlURL, dp.syncInterval)
}

// Stop terminates the background sync loop.
func (dp *DataPlane) Stop() {
	close(dp.stopCh)
}

func (dp *DataPlane) syncLoop() {
	ticker := time.NewTicker(dp.syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-dp.stopCh:
			return
		case <-ticker.C:
			dp.pullConfig()
			dp.pushStatus()
		}
	}
}

func (dp *DataPlane) pullConfig() {
	req, _ := http.NewRequest("GET", dp.controlURL+"/admin/v1/federation/config", nil)
	req.Header.Set("Authorization", "Bearer "+dp.token)
	resp, err := dp.client.Do(req)
	if err != nil {
		log.Printf("federation: config pull failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("federation: config pull returned %d", resp.StatusCode)
		return
	}
	// Read config -- hot-reload integration would go here
	io.ReadAll(resp.Body)
}

func (dp *DataPlane) pushStatus() {
	status := PlaneStatus{
		Name:     dp.name,
		Healthy:  true,
		LastSeen: time.Now(),
	}
	data, _ := json.Marshal(status)
	req, _ := http.NewRequest("POST", dp.controlURL+"/admin/v1/federation/status", bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+dp.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := dp.client.Do(req)
	if err != nil {
		log.Printf("federation: status push failed: %v", err)
		return
	}
	resp.Body.Close()
}

// PushMetrics sends arbitrary metrics data to the control plane.
func (dp *DataPlane) PushMetrics(metricsData []byte) error {
	req, _ := http.NewRequest("POST", dp.controlURL+"/admin/v1/federation/metrics", bytes.NewReader(metricsData))
	req.Header.Set("Authorization", "Bearer "+dp.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := dp.client.Do(req)
	if err != nil {
		return fmt.Errorf("metrics push: %w", err)
	}
	resp.Body.Close()
	return nil
}
