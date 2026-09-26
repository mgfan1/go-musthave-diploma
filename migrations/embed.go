// Package migrations встраивает SQL-миграции схемы базы в бинарь сервиса.
package migrations

import "embed"

// FS содержит файлы миграций в формате golang-migrate.
//
//go:embed *.sql
var FS embed.FS
