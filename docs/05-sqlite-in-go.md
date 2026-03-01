# 05 — SQLite в Go

## Два драйвера: modernc vs mattn

| Аспект | `modernc.org/sqlite` | `mattn/go-sqlite3` |
|--------|---------------------|-------------------|
| CGO | **Нет** (pure Go) | Да (обёртка над C SQLite) |
| Кросс-компиляция | `GOOS=windows go build` — просто работает | Нужен C-компилятор для каждой target platform |
| Build complexity | `go build` | Нужен gcc/clang, проблемы в CI |
| Bulk inserts (1M) | ~5300 ms | ~1500 ms (3.5x быстрее) |
| Простые запросы | ~600 ms | ~350 ms (~1.7x быстрее) |
| Concurrent reads (8 горутин) | **~1600 ms** | ~2830 ms (modernc быстрее!) |

**Наш выбор: `modernc.org/sqlite`** — zero-CGO, простая сборка, приемлемая производительность для debugging-прокси. Для concurrent reads (наш основной кейс) modernc даже быстрее.

---

## `database/sql` — стандартный интерфейс

### `*sql.DB` — это connection pool, не соединение

```go
db, err := sql.Open("sqlite", "file:app.db?_pragma=journal_mode(WAL)")
if err != nil {
    log.Fatal(err)
}
defer db.Close()
// db живёт всё время работы приложения
```

`sql.Open` не открывает соединение. Он создаёт pool, который лениво создаёт connections по мере нужды.

### Почему `*sql.DB` должен быть долгоживущим

- **Pool warming** — уже готовые соединения не нуждаются в повторном открытии
- **Resource management** — open/close на каждый запрос = исчерпание file descriptors
- Создавать `*sql.DB` один раз при старте, закрывать при завершении

### Настройка pool

| Параметр | Дефолт | Назначение |
|----------|--------|-----------|
| `SetMaxOpenConns(n)` | 0 (безлимит) | Макс. открытых соединений |
| `SetMaxIdleConns(n)` | 2 | Сколько держать idle |
| `SetConnMaxLifetime(d)` | 0 (нет) | Макс. время жизни соединения |
| `SetConnMaxIdleTime(d)` | 0 (нет) | Макс. время простоя |

**Для SQLite:** рекомендуется 2-8 соединений, масштабировано по `GOMAXPROCS`. Больше соединений — хуже throughput из-за file-level locking.

---

## WAL Mode — обязателен

По умолчанию SQLite использует rollback journal: **writer блокирует все readers**. WAL (Write-Ahead Logging) инвертирует это:

- **Readers никогда не блокируют writers, writers не блокируют readers**
- Множество читателей работают параллельно пока один writer пишет в WAL
- Критично для Go-приложений где HTTP-хендлеры бегут конкурентно

### Рекомендуемые PRAGMA

```go
pragmas := `
    PRAGMA journal_mode = WAL;
    PRAGMA synchronous = NORMAL;
    PRAGMA cache_size = -32000;        -- 32 MiB page cache
    PRAGMA mmap_size = 20000000000;    -- memory-map до ~20 GB
    PRAGMA temp_store = MEMORY;        -- temp tables в RAM
    PRAGMA foreign_keys = ON;
    PRAGMA busy_timeout = 10000;       -- 10s busy timeout
    PRAGMA optimize;                   -- обновить внутреннюю статистику
`
```

### Нюансы WAL

- **WAL-файл растёт** если всегда есть хотя бы один активный reader — checkpoint не может завершиться. Нужны "окна" без читателей
- **Один writer за раз** — WAL не снимает это ограничение. Конкурентные записи встают в очередь за busy timeout
- **Персистентен** — режим сохраняется в файле БД

---

## Query Builder vs ORM vs Raw SQL

### Raw `database/sql`

```go
rows, err := db.Query("SELECT id, name FROM users WHERE age > ?", 18)
for rows.Next() {
    var id int; var name string
    rows.Scan(&id, &name)
}
```

Полный контроль, нет абстракции. Verbose, нет compile-time validation.

### `squirrel` (наш выбор)

```go
sql, args, err := sq.
    Select("id", "target", "state").
    From("sessions").
    Where(sq.Eq{"state": "active"}).
    OrderBy("created_at DESC").
    ToSql()
```

- **Не ORM** — только строит SQL-строки и аргументы
- Хорош для динамических запросов (фильтры, поиск)
- Нет schema management, нет миграций, нет struct mapping
- Парится с `sqlx` для сканирования результатов в структуры

### `sqlc` (альтернатива)

Пишешь SQL в `.sql` файлах → `sqlc generate` создаёт типобезопасный Go-код:

```sql
-- name: GetSession :one
SELECT id, target, state FROM sessions WHERE id = ?;
```

Генерирует:

```go
func (q *Queries) GetSession(ctx context.Context, id string) (Session, error) { ... }
```

- Compile-time type safety: неверное имя колонки = ошибка компиляции
- Zero runtime overhead
- Ограничение: динамические WHERE-клаузы сложнее

### `GORM` (почему избегаем)

- **Reflection-heavy** — runtime reflection для маппинга структур
- **N+1 проблема** — `Preload` по дефолту делает по запросу на каждый related entity
- **Магия** — auto-migrations, hooks, soft deletes, callbacks могут генерировать неожиданный SQL
- **Сложность отладки** — непонятно какой SQL исполняется

GORM противоречит философии Go: явность и простота.

---

## Scanner и Valuer — кастомные типы в БД

Два интерфейса для прозрачной сериализации/десериализации Go-типов в/из колонок БД.

### `driver.Valuer` — Go → БД

```go
func (d Direction) Value() (driver.Value, error) {
    switch d {
    case ClientToServer:
        return "client_to_server", nil
    case ServerToClient:
        return "server_to_client", nil
    default:
        return nil, fmt.Errorf("invalid direction: %d", d)
    }
}
```

### `sql.Scanner` — БД → Go

```go
func (d *Direction) Scan(src any) error {
    str, ok := src.(string)
    if !ok {
        return fmt.Errorf("expected string for Direction, got %T", src)
    }
    switch str {
    case "client_to_server":
        *d = ClientToServer
    case "server_to_client":
        *d = ServerToClient
    default:
        return fmt.Errorf("unknown direction: %s", str)
    }
    return nil
}
```

**Pointer receiver** у `Scan` обязателен — метод мутирует значение.

Применимо к нашему проекту: `Direction`, `SessionState`, `uuid.UUID` — все кандидаты для Scanner/Valuer.

---

## Prepared Statements

```go
stmt, err := db.Prepare("SELECT target FROM sessions WHERE id = ?")
if err != nil { return err }
defer stmt.Close()

// Переиспользуем stmt множество раз
for _, id := range ids {
    var target string
    err := stmt.QueryRow(id).Scan(&target)
    // ...
}
```

**Под капотом:** prepared statement привязан к конкретному соединению. Если соединение возвращается в pool, `database/sql` перепрепарирует на другом соединении прозрачно.

**Когда использовать:** batch-операции, hot paths. Для одноразовых запросов `db.Query()` внутренне делает prepare+close, и это нормально.

---

## Миграции для SQLite

### goose (рекомендуется)

```bash
goose -dir migrations sqlite3 ./app.db up
```

SQL-файлы с маркерами:

```sql
-- +goose Up
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    target TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'created',
    created_at DATETIME NOT NULL,
    closed_at DATETIME,
    close_code INTEGER
);

-- +goose Down
DROP TABLE sessions;
```

- Встраивается в Go-бинарник через `embed.FS`
- Поддерживает out-of-order миграции
- Go-based миграции для сложных трансформаций данных

### SQLite-специфика

SQLite имеет ограниченный `ALTER TABLE`:
- `DROP COLUMN` только с 3.35.0+
- `RENAME COLUMN` только с 3.25.0+
- Добавить constraints к существующей колонке нельзя

Workaround — 12-step процесс: создать новую таблицу → скопировать данные → удалить старую → переименовать.

---

## Что читать дальше

- **Alex Edwards.** ["Configuring sql.DB for Better Performance."](https://www.alexedwards.net/blog/configuring-sqldb) — тюнинг connection pool
- **jacob.gold.** ["Go + SQLite Best Practices."](https://jacob.gold/posts/go-sqlite-best-practices/) — WAL, pragmas, pool tuning
- **JetBrains Blog.** ["Comparing database/sql, GORM, sqlx, and sqlc."](https://blog.jetbrains.com/go/2023/04/27/comparing-db-packages/) 2023
- **Bytebase.** ["Choose the Right Golang ORM or Query Builder."](https://www.bytebase.com/blog/golang-orm-query-builder/) 2025
- **SQLite docs.** [WAL Mode](https://sqlite.org/wal.html) — primary source
