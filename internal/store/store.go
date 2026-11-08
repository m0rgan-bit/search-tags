// Package store отвечает за чтение тегов из ClickHouse.
package store

import "context"

// Tag — один тег для запроса.
type Tag struct {
	Tag   string `json:"tag"`
	Users uint32 `json:"users"`
}

// TagStore — источник тегов. Интерфейс нужен, чтобы тесты хендлеров не требовали ClickHouse.
type TagStore interface {
	// Tags возвращает теги для уже нормализованного запроса в порядке rank.
	Tags(ctx context.Context, query string) ([]Tag, error)
}
