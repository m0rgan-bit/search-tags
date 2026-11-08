// Package normalize приводит поисковый запрос к каноническому виду.
// Правила совпадают с SQL-функцией normalize_query в sql/01_schema.sql.
package normalize

import "strings"

// Normalize переводит строку в нижний регистр, схлопывает любые подряд идущие
// пробельные символы в один пробел и обрезает пробелы по краям.
func Normalize(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
