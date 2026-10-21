package siuidata

import _ "embed"

//go:embed dashboard.html
var dashboard string

//go:embed settings.html
var settingsPage string

//go:embed versions.html
var versionsPage string

//go:embed debug.html
var debugPage string

//go:embed clients.html
var clientsPage string
