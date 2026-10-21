// Package migrations embeds the SQL migration files so the server binary can
// apply them without shipping the directory separately.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
