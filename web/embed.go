package web

import "embed"

//go:embed *.html *.css *.js fonts/*.woff2 lib/*.js lib/*.css
var Files embed.FS
