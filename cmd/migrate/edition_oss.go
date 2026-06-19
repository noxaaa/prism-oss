package main

import (
	"fmt"

	"github.com/noxaaa/prism-oss/pkg/edition"
)

func migrationProviderForKey(key edition.Key) (edition.Provider, error) {
	if key != edition.KeyOSS {
		return nil, fmt.Errorf("cmd/migrate oss build requires PRISM_EDITION=oss or unset; use the regular build target for PRISM_EDITION=full")
	}
	return edition.ProviderForKey(key)
}
