package db

import "embed"

const SQLRoot = "migrations"

//go:embed migrations
var SQLFiles embed.FS
