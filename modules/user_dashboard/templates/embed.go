package templates

import "embed"

//go:embed dashboard.html team.html overdue.html
var FS embed.FS
