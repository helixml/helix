package version

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/data"
)

type PingService struct {
	db            *store.PostgresStore
	launchpadHost string
	ticker        *time.Ticker
	done          chan bool
}

func NewPingService(db *store.PostgresStore) *PingService {
	return &PingService{
		db:            db,
		launchpadHost: "https://deploy.helix.ml",
		ticker:        time.NewTicker(1 * time.Hour),
		done:          make(chan bool),
	}
}

func (s *PingService) Start(ctx context.Context) {
	go func() {
		// Send initial ping
		s.sendPing()

		for {
			select {
			case <-s.done:
				return
			case <-ctx.Done():
				return
			case <-s.ticker.C:
				s.sendPing()
			}
		}
	}()
}

func (s *PingService) Stop() {
	s.ticker.Stop()
	s.done <- true
}

func (s *PingService) sendPing() {
	// Get app count from database
	appCount, err := s.db.GetAppCount()
	if err != nil {
		log.Printf("Error getting app count: %v", err)
		return
	}

	// Get user count from Keycloak
	userCount, err := s.getUserCount()
	if err != nil {
		log.Printf("Error getting user count: %v", err)
		return
	}

	// Prepare ping data
	pingData := map[string]interface{}{
		"version":     data.Version,
		"apps_count":  appCount,
		"users_count": userCount,
	}

	// Send ping to launchpad
	jsonData, err := json.Marshal(pingData)
	if err != nil {
		log.Printf("Error marshaling ping data: %v", err)
		return
	}

	resp, err := http.Post(
		fmt.Sprintf("%s/api/ping", s.launchpadHost),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		log.Printf("Error sending ping: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		log.Printf("Unexpected status code from ping: %d", resp.StatusCode)
	}
}

func (s *PingService) getUserCount() (int, error) {
	// TODO: Implement Keycloak user count
	// This would need to use the Keycloak admin API to get the user count
	// For now returning a placeholder
	return 1, nil
}
