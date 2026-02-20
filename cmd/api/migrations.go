package main

import (
	"embed"
	"io/fs"
)

//go:embed migrations/*.sql
var migrations embed.FS

func getMigrationsFS() fs.FS {
	fs, _ := fs.Sub(migrations, "migrations")
	return fs
}
