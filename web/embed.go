// Package web berisi template HTML dan file statis yang di-embed ke dalam binary,
// jadi saat deploy cukup upload satu file executable.
package web

import "embed"

//go:embed templates static
var FS embed.FS
