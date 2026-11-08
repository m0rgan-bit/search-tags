# Поисковые теги

Под строкой поиска показываем до 10 самых частых «следующих» запросов. Теги считаются в ClickHouse по логам поиска, отдаются Go-сервисом (`GET /api/tags?q=...`) и показываются на простой веб-странице.

## Как запустить

Нужны Docker и Go 1.22+.

```bash
# 1. ClickHouse (logs.parquet уже лежит в data/ и монтируется в контейнер)
docker compose up -d

# удобный алиас для клиента
CH="docker compose exec -T clickhouse clickhouse-client --user app --password app --database tags_db --multiquery"

# 2. Таблицы, загрузка данных, построение тегов
$CH < sql/01_schema.sql
$CH < sql/02_load_data.sql
$CH < sql/00_explore.sql
$CH < sql/03_build_tags.sql

# 3. SQL-проверки (в конце должно быть ALL CHECKS PASSED)
./scripts/run_checks.sh

# 4. Go-сервис
go mod tidy                   # один раз: скачает clickhouse-go и создаст go.sum
go run ./cmd/tagsvc
# открыть http://localhost:8080/ или:
curl 'http://localhost:8080/api/tags?q=платье'

# 5. Go-тесты (ClickHouse не нужен)
go test ./... -v
```

Переменные окружения сервиса (значения по умолчанию подходят под `docker-compose.yml`):

| Переменная | По умолчанию | Что это |
|---|---|---|
| `CLICKHOUSE_ADDR` | `localhost:9000` | адрес и порт ClickHouse |
| `CLICKHOUSE_DB` | `tags_db` | база |
| `CLICKHOUSE_USER` / `CLICKHOUSE_PASSWORD` | `app` / `app` | доступ |
| `HTTP_PORT` | `8080` | порт сервиса |
| `QUERY_TIMEOUT` | `3s` | таймаут запроса к базе |

## Структура

```
sql/00_explore.sql     разведка грязных данных
sql/01_schema.sql      функция normalize_query и таблицы search_logs, tags
sql/02_load_data.sql   загрузка logs.parquet
sql/03_build_tags.sql  построение тегов
sql/04_checks.sql      SQL-проверки (0 нарушений = всё хорошо)
cmd/tagsvc             main
internal/normalize     нормализация запроса (+ unit-тест)
internal/store         чтение тегов из ClickHouse
internal/server        хендлеры, шаблон html/template, тесты
```

## Почему такая схема таблиц

**`search_logs`: `MergeTree`, `ORDER BY (user_id, timestamp)`.**
Весь расчёт строится на последовательности запросов одного пользователя: нужно сгруппировать по `user_id` и отсортировать по времени. Если порядок хранения совпадает с этим, ClickHouse читает данные уже сгруппированными, а оконные функции работают без тяжёлой пересортировки. Партиции не делал: данных немного, а лишние партиции только усложняют.

**Типы.** `timestamp` — `DateTime` (секундной точности достаточно, порог в 30 минут),    `user_id` — `String`: в parquet это число (`UInt64`), но строка принимает id любого вида и работает так же; при желании колонку можно заменить на `UInt64`. `search_query` — `String`.

**`tags`: `MergeTree`, `ORDER BY (query, rank)`.**
Единственный запрос сервиса — `WHERE query = ? ORDER BY rank`. Первичный ключ `(query, rank)` даёт быстрый поиск по `query` и отдаёт строки сразу в порядке `rank`. `rank` — `UInt8` (1–10), `users` — `UInt32`.

## Допущения по «грязным» данным

- **Что показала разведка данных (`00_explore.sql`):** в логах 992 318 строк от 50 078 пользователей за 12–18.10.2026. Пустых запросов, пустых user_id, дат из будущего и полных дубликатов нет (схлопывание дубликатов оставлено как защитная мера). Главная «грязь»: у 89 365 строк (около 9%) запрос написан с другим регистром или лишними пробелами, поэтому нормализация обязательна.
- **Нормализация** одна и та же в SQL (`normalize_query`) и в Go: нижний регистр (в SQL именно `lowerUTF8`, обычный `lower` не трогает кириллицу), любые пробельные символы, включая неразрывный пробел и табы, схлопываются в один пробел, края обрезаются.
- Запросы, пустые после нормализации, и строки с пустым `user_id` выбрасываются **до** построения пар.
- Полные дубликаты (тот же пользователь, то же время, тот же текст) схлопываются: это повторная запись события, а не два действия пользователя.
- Порядок запросов внутри одной секунды у одного пользователя неопределён, поэтому сортируем по `(timestamp, query)`, чтобы результат был воспроизводимым.
- «Не больше 30 минут» понимается как `<= 1800` секунд.
- Уникальных пользователей считаем через `uniqExact`, а не `uniq`: `uniq` приблизительный, и порог «не меньше 5» мог бы работать неточно.
- При равном `users` теги упорядочиваются по алфавиту, чтобы `rank` был детерминированным.
- Два подозрительно активных пользователя (90000001 и 90000002, 1000 и 600 запросов при максимуме 273 у остальных) оказались ботами: каждый чередует одну и ту же пару запросов («платье» ↔ «странный запрос» и «кроссовки» ↔ «кроссовки nike акция»). Из расчёта они не исключены намеренно: правило задания считать уникальных пользователей, а не события, ограничивает вклад каждого бота одним голосом на пару, а порог в 5 пользователей он в одиночку набрать не может. Если бы считали события, «странный запрос» стал бы тегом №1 для «платья» (500 повторов). Проверка: запрос к таблице tags по «странный запрос» возвращает 0 строк.
- Время в исходном файле хранится с точностью до миллисекунд, но во всех 992 318 строках миллисекундная часть равна нулю, поэтому округление до секунд (`DateTime`) ничего не теряет.

## Сервис

- `GET /api/tags?q=<текст>` → `{"query":"платье","tags":[{"tag":"...","users":12}, ...]}` в порядке `rank`.
- Пустой `q` (или только пробелы) → `400`. Нет тегов → `200` и `"tags":[]`. Ошибка базы → `500`, таймаут → `504`.
- `q` нормализуется тем же способом, что и в SQL, поэтому «Платье » и «платье» дают одинаковый ответ.
- SQL к ClickHouse передаёт значение только именованным параметром (`@q`), строки не склеиваются.
- Веб-страница на `html/template`: экранирование включено, в том числе для тегов из базы (в логах лежит текст пользователей). Тег — ссылка вида `/?q=<тег>`, клик подставляет его в поле и показывает теги для него.

## Результаты проверок

### SQL-проверки
```
$ ./scripts/run_checks.sh
check   violations
variants_return_different_tags  0
query_equals_tag        0
rank_outside_1_10       0
users_below_5   0
users_not_descending_by_rank    0
rank_not_contiguous_from_1      0
not_normalized_or_empty 0
duplicate_query_tag_pairs       0
more_than_10_tags_per_query     0
ALL CHECKS PASSED
```  
### Go-тесты
```
$ go test ./... -v
?       searchtags/cmd/tagsvc   [no test files]
?       searchtags/internal/config      [no test files]
=== RUN   TestNormalize
=== RUN   TestNormalize/already_normal
=== RUN   TestNormalize/upper_case_cyrillic
=== RUN   TestNormalize/trailing_space
=== RUN   TestNormalize/leading_and_trailing
=== RUN   TestNormalize/inner_repeated_spaces
=== RUN   TestNormalize/tabs_and_newlines_inside
=== RUN   TestNormalize/non-breaking_space
=== RUN   TestNormalize/mixed_everything
=== RUN   TestNormalize/yo
=== RUN   TestNormalize/latin
=== RUN   TestNormalize/empty
=== RUN   TestNormalize/only_spaces
--- PASS: TestNormalize (0.00s)
    --- PASS: TestNormalize/already_normal (0.00s)
    --- PASS: TestNormalize/upper_case_cyrillic (0.00s)
    --- PASS: TestNormalize/trailing_space (0.00s)
    --- PASS: TestNormalize/leading_and_trailing (0.00s)
    --- PASS: TestNormalize/inner_repeated_spaces (0.00s)
    --- PASS: TestNormalize/tabs_and_newlines_inside (0.00s)
    --- PASS: TestNormalize/non-breaking_space (0.00s)
    --- PASS: TestNormalize/mixed_everything (0.00s)
    --- PASS: TestNormalize/yo (0.00s)
    --- PASS: TestNormalize/latin (0.00s)
    --- PASS: TestNormalize/empty (0.00s)
    --- PASS: TestNormalize/only_spaces (0.00s)
=== RUN   TestNormalizeIdempotent
--- PASS: TestNormalizeIdempotent (0.00s)
PASS
ok      searchtags/internal/normalize   (cached)
=== RUN   TestAPIEmptyQueryReturns400
--- PASS: TestAPIEmptyQueryReturns400 (0.04s)
=== RUN   TestAPIDoesNotHitStoreOnBadQuery
--- PASS: TestAPIDoesNotHitStoreOnBadQuery (0.00s)
=== RUN   TestAPINormalizationGivesSameAnswer
--- PASS: TestAPINormalizationGivesSameAnswer (0.00s)
=== RUN   TestAPIReturnsTagsInOrder
--- PASS: TestAPIReturnsTagsInOrder (0.00s)
=== RUN   TestAPINoTagsReturnsEmptyListAnd200
--- PASS: TestAPINoTagsReturnsEmptyListAnd200 (0.00s)
=== RUN   TestAPIStoreErrorReturns500
--- PASS: TestAPIStoreErrorReturns500 (0.00s)
=== RUN   TestAPITimeoutReturns504
--- PASS: TestAPITimeoutReturns504 (0.02s)
=== RUN   TestPageShowsEmptyMessage
--- PASS: TestPageShowsEmptyMessage (0.00s)
=== RUN   TestPageTagIsClickableLink
--- PASS: TestPageTagIsClickableLink (0.00s)
=== RUN   TestPageEscapesUserInputAndTags
--- PASS: TestPageEscapesUserInputAndTags (0.00s)
=== RUN   TestPageEmptyQueryReturns400
--- PASS: TestPageEmptyQueryReturns400 (0.00s)
=== RUN   TestPageWithoutQueryIsOK
--- PASS: TestPageWithoutQueryIsOK (0.00s)
PASS
ok      searchtags/internal/server      0.909s
?       searchtags/internal/store       [no test files]
```