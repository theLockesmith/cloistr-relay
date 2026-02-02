package web

import "embed"

// Templates contains all HTML templates for the admin UI
//
//go:embed templates/*.html templates/partials/*.html
var Templates embed.FS

// Static contains all static assets (JS, CSS) for the admin UI
//
//go:embed static/js/*.js
var Static embed.FS
