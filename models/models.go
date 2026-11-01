// Package models embeds the demo NetLogo programs shipped with GoNetLogo.
package models

import "embed"

// FS contains the demo models (fire, life, termites, wolf sheep, output, edge, plot)
// and the Models Library sample Robby the Robot. The bundled library under
// library/ is not embedded; the server and the Open dialog read it from disk.
//
//go:embed *.nlogo
var FS embed.FS
