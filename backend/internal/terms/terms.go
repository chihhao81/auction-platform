// Package terms holds the current user terms. The version is defined next to
// the text it describes, so changing the text forces a deliberate version bump
// and every user must accept the new version before using protected APIs.
package terms

import _ "embed"

// CurrentVersion identifies terms.md. Bump it whenever terms.md changes.
const CurrentVersion = "2026-10-21"

//go:embed terms.md
var Content string
