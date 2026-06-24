package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/noxaaa/prism-oss/pkg/core/auth"
	"github.com/noxaaa/prism-oss/pkg/core/config"
	"github.com/noxaaa/prism-oss/pkg/core/handler"
	"github.com/noxaaa/prism-oss/pkg/core/repo"
	"github.com/noxaaa/prism-oss/pkg/core/service"
	"github.com/noxaaa/prism-oss/pkg/edition"
)

func main() {
	cfg, err := config.LoadControlPlane()
	if err != nil {
		log.Fatal(err)
	}
	secret := []byte(cfg.ControlPlaneInternalKey)
	if len(secret) == 0 {
		log.Fatal("CONTROL_PLANE_INTERNAL_JWT_SECRET is required")
	}
	db, err := repo.OpenPostgres(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()
	store := repo.NewPostgresStore(db)
	controlEdition, err := ossControlPlaneEdition()
	if err != nil {
		log.Fatal(err)
	}

	server := handler.NewControlServer(handler.ControlServerOptions{
		TokenVerifier:           auth.HMACInternalTokenSigner{Secret: secret},
		WebUserVerifier:         auth.HMACWebUserTokenSigner{Secret: secret},
		RepositoryStore:         store,
		AppName:                 cfg.AppName,
		ControlPlaneURL:         cfg.ControlPlaneURL,
		AgentReleaseVersion:     cfg.AgentReleaseVersion,
		AgentTokenSigningSecret: []byte(cfg.AgentTokenSigningSecret),
		DNSSecretEncryptionKey:  cfg.DNSSecretEncryptionKey,
		GeoIPResolver:           service.NewGeoIPResolver(cfg.GeoIPDBPath),
		Edition:                 controlEdition,
	})

	address := envOrDefault("CONTROL_PLANE_HTTP_ADDR", ":8080")
	log.Printf("%s control-plane listening on %s", cfg.AppName, address)
	if err := http.ListenAndServe(address, server); err != nil {
		log.Fatal(err)
	}
}

func ossControlPlaneEdition() (edition.Provider, error) {
	key, err := edition.KeyFromString(os.Getenv("PRISM_EDITION"))
	if err != nil {
		return nil, err
	}
	if key != edition.KeyOSS {
		return nil, fmt.Errorf("cmd/control-plane-oss requires PRISM_EDITION=oss or unset; use the regular build target for PRISM_EDITION=full")
	}
	return edition.ProviderForKey(key)
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
