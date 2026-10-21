// Package models embeds the demo NetLogo programs shipped with GoNetLogo.
package models

import "embed"

// FS contains the demo models (fire, life, termites, wolf sheep, output, edge)
// and the Models Library sample Robby the Robot.
//
//go:embed *.nlogo
var FS embed.FS
