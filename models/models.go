// Package models embeds the demo NetLogo programs shipped with GoNetLogo.
package models

import "embed"

// FS contains the original demo models (fire, life, termites, wolf sheep, output, edge).
//
//go:embed *.nlogo
var FS embed.FS
