package migrations

import "embed"

// FS contains the OSS auth and core goose migrations for downstream editions.
//
//go:embed auth/*.sql core/*.sql
var FS embed.FS
