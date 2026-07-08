// Package ui embebe las plantillas HTML y los archivos estáticos en el
// binario, de modo que el deploy sea un solo ejecutable.
package ui

import "embed"

//go:embed templates static
var Files embed.FS
