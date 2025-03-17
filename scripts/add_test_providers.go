// This script adds test OAuth providers to the database
package main

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func main() {
	// Load config
	cfg, err := config.LoadServerConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to database
	postgresStore, err := store.NewPostgresStore(cfg.Store)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	ctx := context.Background()

	// Define test providers
	providers := []*types.OAuthProvider{
		{
			ID:           uuid.New().String(),
			Name:         "GitHub (Test)",
			Description:  "Connect with your GitHub account",
			Type:         types.OAuthProviderTypeGitHub,
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			CallbackURL:  "http://localhost:8080/oauth/flow/callback",
			AuthURL:      "https://github.com/login/oauth/authorize",
			TokenURL:     "https://github.com/login/oauth/access_token",
			UserInfoURL:  "https://api.github.com/user",
			CreatorID:    "system",
			CreatorType:  types.OwnerTypeSystem,
			Scopes:       []string{"user:email", "read:user"},
			Enabled:      true,
		},
		{
			ID:           uuid.New().String(),
			Name:         "Google (Test)",
			Description:  "Connect with your Google account",
			Type:         types.OAuthProviderTypeGoogle,
			ClientID:     "test-google-client-id",
			ClientSecret: "test-google-client-secret",
			CallbackURL:  "http://localhost:8080/oauth/flow/callback",
			AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:     "https://oauth2.googleapis.com/token",
			UserInfoURL:  "https://www.googleapis.com/oauth2/v3/userinfo",
			CreatorID:    "system",
			CreatorType:  types.OwnerTypeSystem,
			Scopes:       []string{"profile", "email"},
			Enabled:      true,
		},
	}

	// Add providers to database
	for _, provider := range providers {
		_, err := postgresStore.CreateOAuthProvider(ctx, provider)
		if err != nil {
			log.Printf("Failed to create provider %s: %v", provider.Name, err)
			continue
		}
		log.Printf("Created provider: %s", provider.Name)
	}

	log.Println("Done!")
}
