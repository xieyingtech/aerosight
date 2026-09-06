package migrations

import "embed"

// Files contains byte-for-byte copies produced by scripts/prepare-server.mjs.
//
//go:embed sql/*.sql
var Files embed.FS
