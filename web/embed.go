package web

import "embed"

// Templates contains all HTML templates for the admin UI
//
//go:embed templates/*.html templates/partials/*.html
var Templates embed.FS

// Static contains all static assets (JS, CSS, icons) for the admin UI
//
//go:embed static/*.ico static/*.svg static/*.png static/js/*.js
var Static embed.FS
