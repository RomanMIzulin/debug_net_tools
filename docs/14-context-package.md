# 14 — context.Context: Cancellation, Deadlines, Request Scope

## Зачем существует `context`

В серверном Go каждый запрос порождает дерево горутин: HTTP handler → proxy relay → DB query → pub/sub publish. Когда клиент отключается (или admin нажимает Ctrl+C), **все** горутины в дереве должны корректно завершиться. Без единого механизма отмены — горутины утекают.

`context.Context` (пакет `context`, stdlib с Go 1.7) решает три задачи:

1. **Cancellation propagation** — отмена каскадирует от родителя к потомкам
2. **Deadline/timeout enforcement** — ограничение времени операции
3. **Request-scoped values** — пробрасывание метаданных (trace ID, auth) без засорения сигнатур

---

## Интерфейс

```go
type Context interface {
    Deadline() (deadline time.Time, ok bool)
    Done() <-chan struct{}
    Err() error
    Value(key any) any
}
```

Четыре метода, все read-only. Context **неизменяем** после создания — нет `Set`, `Cancel`, `Extend`. Создание нового — через функции-конструкторы, которые оборачивают родительский context.

| Метод | Что возвращает |
|-------|---------------|
| `Done()` | Канал, закрываемый при отмене. `select { case <-ctx.Done(): }` |
| `Err()` | `nil` пока активен; `context.Canceled` или `context.DeadlineExceeded` после отмены |
| `Deadline()` | Время дедлайна, если установлен |
| `Value(key)` | Значение по ключу, или nil |

---

## Конструкторы

### Корневые context'ы

```go
ctx := context.Background() // корень дерева, никогда не отменяется
ctx := context.TODO()       // placeholder — "тут должен быть context, но пока не решили какой"
```

`Background()` — для `main()`, инициализации, тестов. `TODO()` — маркер для рефакторинга, grep-able: `grep -r "context.TODO"` показывает где context ещё не проброшен.

### Производные context'ы

Каждый конструктор принимает **parent** и возвращает **child + cancel**:

```go
// Ручная отмена (graceful shutdown, клиент отключился)
ctx, cancel := context.WithCancel(parent)
defer cancel()

// Таймаут (сетевая операция не должна длиться вечно)
ctx, cancel := context.WithTimeout(parent, 30*time.Second)
defer cancel()

// Абсолютный дедлайн (SLA: ответ до 14:00 UTC)
ctx, cancel := context.WithDeadline(parent, deadline)
defer cancel()

// Значение (trace ID, auth token)
ctx := context.WithValue(parent, traceIDKey, "abc-123")
```

**`defer cancel()` обязателен** — предотвращает утечку горутины внутри context runtime. Даже если context истечёт по таймауту, cancel освобождает ресурсы раньше.

### Cause (Go 1.20+)

```go
ctx, cancel := context.WithCancelCause(parent)
cancel(fmt.Errorf("upstream server closed connection"))
// ...
context.Cause(ctx) // → "upstream server closed connection"
```

`WithCancelCause` добавляет **причину** отмены. `context.Cause(ctx)` возвращает причину вместо generic `context.Canceled`. Критично для диагностики: "почему proxy relay завершился?" — "потому что upstream закрыл соединение", а не просто "cancelled".

### AfterFunc (Go 1.21+)

```go
stop := context.AfterFunc(ctx, func() {
    conn.Close() // вызовется когда ctx отменён
})
// stop() отменяет callback если контекст ещё жив
```

Регистрирует callback на отмену. Альтернатива горутине `go func() { <-ctx.Done(); cleanup() }()`.

---

## Дерево context'ов

Context'ы формируют **дерево**. Отмена родителя каскадирует на всех потомков:

```
context.Background()
└── WithCancel (server lifetime)
    ├── WithCancel (connection 1)
    │   ├── WithTimeout (relay c→s, 30s)
    │   └── WithTimeout (relay s→c, 30s)
    ├── WithCancel (connection 2)
    │   └── ...
    └── WithTimeout (DB query, 5s)
```

Когда server lifetime context отменяется (Ctrl+C) — **все** соединения и запросы каскадно завершаются. Когда одно соединение закрывается — только его потомки.

**Отмена child НЕ влияет на parent.** `cancel()` дочернего не трогает родительский — это одностороннее распространение.

---

## Конвенции

### 1. Первый параметр

```go
// ПРАВИЛЬНО
func (s *Store) GetSession(ctx context.Context, id uuid.UUID) (*Session, error)
func relay(ctx context.Context, client, server *websocket.Conn) error

// НЕПРАВИЛЬНО
func (s *Store) GetSession(id uuid.UUID, ctx context.Context) (*Session, error)
func relay(client, server *websocket.Conn, ctx context.Context) error
```

Context — **всегда первый параметр**, именуется `ctx`. Единственное исключение: методы, чей receiver уже содержит context (например, `http.Request` — `r.Context()`).

### 2. Не хранить в структурах

```go
// НЕПРАВИЛЬНО — context привязан к lifecycle запроса, не объекта
type ProxyConn struct {
    ctx    context.Context // ← антипаттерн
    client *websocket.Conn
}

// ПРАВИЛЬНО — передавать через параметры методов
func (p *ProxyConn) Relay(ctx context.Context) error { ... }
```

Хранение context в struct размывает ownership: кто отменяет? когда? Context привязан к **операции** (запрос, соединение), а не к объекту.

**Исключение из правила:** `http.Request` хранит context (`r.Context()`). Но Request сам является per-request объектом с определённым lifecycle.

### 3. Не передавать nil

```go
// НЕПРАВИЛЬНО
store.GetSession(nil, id)

// ПРАВИЛЬНО — если не знаешь какой context, используй TODO
store.GetSession(context.TODO(), id)
```

`context.TODO()` — явный сигнал "здесь нужен настоящий context, но пока не решили".

### 4. WithValue — только для request-scoped metadata

```go
// ПРАВИЛЬНО — trace ID, request ID, auth token
type ctxKey string
const traceIDKey ctxKey = "traceID"
ctx = context.WithValue(ctx, traceIDKey, "abc-123")

// НЕПРАВИЛЬНО — optional parameters, config, dependencies
ctx = context.WithValue(ctx, "db", database)     // ← dependency injection через context
ctx = context.WithValue(ctx, "verbose", true)     // ← config через context
```

`WithValue` — для metadata, которая проходит через весь call chain (tracing, auth). Для dependency injection — конструкторы. Для config — параметры.

**Type-safe ключи:** всегда использовать неэкспортированный тип для ключей, чтобы избежать коллизий между пакетами.

---

## Context в сетевых серверах

### HTTP Server

```go
func handler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context() // context от HTTP server, отменяется при disconnect клиента

    session, err := store.GetSession(ctx, id)
    if err != nil {
        if errors.Is(err, context.Canceled) {
            return // клиент ушёл, ответ не нужен
        }
        http.Error(w, err.Error(), 500)
        return
    }
}
```

`net/http` server автоматически создаёт context для каждого request и отменяет его когда:
- Клиент закрывает соединение
- `http.Server.Shutdown()` вызван
- Request body полностью прочитан и handler завершился

### Graceful Shutdown

```go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    srv := &http.Server{Addr: ":8080"}

    go func() {
        <-ctx.Done()
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()
        srv.Shutdown(shutdownCtx)
    }()

    srv.ListenAndServe()
}
```

`signal.NotifyContext` (Go 1.16+) — создаёт context, отменяемый при получении OS-сигнала. Объединяет signal handling с context tree.

**Паттерн двойного Ctrl+C:**

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()

go func() {
    <-ctx.Done()
    stop() // reset signal handler
    // второй Ctrl+C → дефолтное поведение (немедленное завершение)

    shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    log.Println("shutting down gracefully, press Ctrl+C again to force")
    srv.Shutdown(shutdownCtx)
}()
```

---

## Паттерны использования

### Проверка отмены в циклах

```go
func processFrames(ctx context.Context, frames <-chan Frame) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case f, ok := <-frames:
            if !ok {
                return nil // канал закрыт
            }
            if err := handle(f); err != nil {
                return fmt.Errorf("handle frame: %w", err)
            }
        }
    }
}
```

### Context для database operations

```go
func (s *Store) GetSession(ctx context.Context, id uuid.UUID) (*Session, error) {
    query, args, err := sq.
        Select("id", "target", "state").
        From("sessions").
        Where(sq.Eq{"id": id.String()}).
        ToSql()
    if err != nil {
        return nil, fmt.Errorf("build query: %w", err)
    }

    row := s.db.QueryRowContext(ctx, query, args...)
    // ...
}
```

`database/sql` имеет `*Context`-варианты для всех операций: `QueryContext`, `QueryRowContext`, `ExecContext`, `PrepareContext`. **Всегда использовать Context-варианты** — без них запрос не отменится при shutdown.

### Cascading cancellation в relay

```go
func relay(ctx context.Context, client, server *websocket.Conn) error {
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()

    errc := make(chan error, 2)

    go func() {
        errc <- pump(ctx, client, server, core.ClientToServer)
    }()
    go func() {
        errc <- pump(ctx, server, client, core.ServerToClient)
    }()

    select {
    case err := <-errc:
        cancel() // отменяем вторую горутину
        return err
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

Когда одна сторона relay падает, `cancel()` каскадно останавливает другую. Без context вторая горутина висела бы на `ReadMessage()` до TCP-таймаута.

---

## Context в проекте wsproxy

### Где context должен пропагироваться

```
main()
  └── context.Background() + signal.NotifyContext
      ├── HTTP server (per-request context)
      │   └── WebSocket upgrade
      │       └── relay(ctx, ...)
      │           ├── pump c→s (ctx)
      │           └── pump s→c (ctx)
      ├── Store operations
      │   ├── SaveFrame(ctx, ...)
      │   ├── GetSession(ctx, ...)
      │   └── ListSessions(ctx, ...)
      ├── Bus.Subscribe(ctx)
      │   └── context отмена → автоматическая отписка
      └── Graceful shutdown
          └── WithTimeout(10s) для drain
```

### Текущее состояние (TODO)

Текущий код **не использует context** — это критический пробел:

- `proxy/server.go`: структуры `ClientConn`, `TargetConn` не принимают context
- `storage/store.go`: `Store` не имеет методов с context
- `cmd/serve.go`: `RunE` не создаёт корневой context и не обрабатывает сигналы
- `core/event.go`: domain types не затронуты (context не хранится в них — это правильно)

Первым шагом: добавить context в `Store` методы и `relay()`.

---

## Антипаттерны

### 1. Ignore context

```go
// ПЛОХО — context передан но не используется
func (s *Store) GetSession(ctx context.Context, id uuid.UUID) (*Session, error) {
    row := s.db.QueryRow("SELECT ...", id) // QueryRow, не QueryRowContext!
    // ...
}
```

Если принимаешь context — **используй его**. Иначе это ложная безопасность.

### 2. Context в горячем пути без необходимости

```go
// ПЛОХО — проверка Done() на каждом байте
for i := 0; i < len(payload); i++ {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }
    process(payload[i])
}
```

Context проверяют на **операциях** (сетевой вызов, DB-запрос, итерация по каналу), а не на каждом шаге CPU-bound цикла.

### 3. Продление жизни context'а

```go
// ПЛОХО — background горутина использует request context
func handler(w http.ResponseWriter, r *http.Request) {
    go saveAsync(r.Context(), data) // context отменится когда handler вернётся!
}

// ПРАВИЛЬНО — фоновая работа использует свой context
func handler(w http.ResponseWriter, r *http.Request) {
    bgCtx := context.WithoutCancel(r.Context()) // Go 1.21+: наследует values, но не cancellation
    go saveAsync(bgCtx, data)
}
```

`context.WithoutCancel` (Go 1.21+) — создаёт context который наследует values (trace ID), но не привязан к lifecycle родителя.

---

## Сравнение с Python

| Аспект | Go `context` | Python `asyncio` |
|--------|-------------|------------------|
| Отмена | `ctx.Done()` канал | `task.cancel()` + `CancelledError` |
| Таймаут | `context.WithTimeout` | `asyncio.wait_for(coro, timeout)` |
| Каскадная отмена | Встроена (дерево context'ов) | `TaskGroup` (Python 3.11+) |
| Request scope values | `context.WithValue` | `contextvars.ContextVar` (PEP 567) |
| Graceful shutdown | `signal.NotifyContext` | `asyncio.Runner` + signal handlers |
| Первый параметр | Конвенция (не enforce) | N/A (неявно через event loop) |

### Python `contextvars` (PEP 567, Python 3.7+)

```python
import contextvars

request_id: contextvars.ContextVar[str] = contextvars.ContextVar('request_id')

async def handler(req):
    request_id.set(req.id)
    await process()  # process() видит request_id без параметра

async def process():
    rid = request_id.get()  # доступно через ContextVar
```

Python `contextvars` решает ту же задачу что `context.WithValue` — пробрасывание request-scoped данных. Но в Python это **implicit** (через корутину), а в Go — **explicit** (через параметр). Go подход verbose но безопаснее: видно в сигнатуре что функция context-aware.

### Python `asyncio.TaskGroup` (Python 3.11+)

```python
async with asyncio.TaskGroup() as tg:
    tg.create_task(pump_c2s(ws_client, ws_server))
    tg.create_task(pump_s2c(ws_server, ws_client))
# выход из with → все задачи отменены
```

Ближайший аналог Go context tree для asyncio. Отмена одной задачи → отмена всей группы.

---

## Внутреннее устройство

### Как работает `Done()`

`Done()` возвращает канал, который закрывается при отмене. Закрытие канала — **broadcast**: все горутины, делающие `<-ctx.Done()`, разблокируются одновременно. Это ключевое свойство — один `cancel()` будит N горутин.

### Как каскадирует отмена

При создании `WithCancel(parent)` runtime регистрирует child в parent'е. Когда parent отменяется:

```
parent.cancel()
  → закрыть parent.done канал
  → для каждого child в parent.children:
      child.cancel()
        → закрыть child.done канал
        → рекурсивно для child.children
  → удалить parent из своего parent'а
```

Гарантия: отмена child **до** возврата из parent `cancel()`.

### Стоимость

| Операция | Стоимость |
|----------|-----------|
| `context.Background()` | ~0 (singleton) |
| `context.WithCancel()` | 1 аллокация, ~200 ns |
| `context.WithValue()` | 1 аллокация, O(N) lookup по цепочке |
| `<-ctx.Done()` в select | ~0 (closed channel read = nonblocking) |

`WithValue` — **linked list** lookup, O(N) от глубины цепочки. Не использовать для частых lookups. Для hot paths: достать значение один раз в начале функции.

---

## Теоретические истоки

### CML — Concurrent ML (Reppy, 1991)

`context.Done()` — канал как **event** — восходит к **Concurrent ML** (John Reppy, "Concurrent Programming in ML", Cambridge, 1999). CML events — first-class синхронные объекты, которые можно комбинировать через `select`. Go `select` + закрытие канала = broadcast event, что в CML реализуется через `Event.sync`.

### Structured Concurrency (Martin Sústrik, 2016)

Context tree — ранняя форма **structured concurrency**: горутины организованы в иерархию, lifetime потомков ограничен lifetime'ом родителя.

Sústrik (автор ZeroMQ/nanomsg) ввёл термин в ["Structured Concurrency"](https://250bpm.com/blog:71/), 2016. Затем:
- **Trio** (Python, Nathaniel Smith, 2017) — nurseries / cancel scopes
- **JEP 453** (Java 21, 2023) — structured concurrency API
- **Swift concurrency** (2021) — task groups

Go context — **partial structured concurrency**: cancellation каскадирует, но нет enforce что горутина завершится до parent (в отличие от Trio nurseries). `errgroup.Group` (golang.org/x/sync) добавляет "wait for all children" семантику, приближая Go к полной structured concurrency.

### Capability-Passing Style

`context.Context` как первый параметр — пример **capability-passing**: функция явно объявляет какие "возможности" ей нужны (deadline, cancellation, values). Это из **object-capability model** (Dennis & Van Horn, "Programming Semantics for Multiprogrammed Computations", CACM, 1966): доступ к ресурсу только через явно переданный capability-токен.

Сравни с Python `contextvars` (ambient authority — неявный доступ через глобальный механизм).

---

## Что читать дальше

### Обязательное
- **Go Blog.** ["Go Concurrency Patterns: Context."](https://go.dev/blog/context) Sameer Ajmani, 2014 — каноническое введение
- **Go Blog.** ["Contexts and structs."](https://go.dev/blog/context-and-structs) Jean de Klerk, Matt T. Proud, 2021 — почему не хранить в struct
- **Go Package docs.** [context](https://pkg.go.dev/context) — reference

### Structured Concurrency
- **Sústrik, M.** ["Structured Concurrency."](https://250bpm.com/blog:71/) 2016
- **Smith, N.J.** ["Notes on structured concurrency, or: Go statement considered harmful."](https://vorpus.org/blog/notes-on-structured-concurrency-or-go-statement-considered-harmful/) 2018

### Углублённое
- **Reppy, J.H.** *Concurrent Programming in ML.* Cambridge University Press, 1999
- **Dennis, J.B. & Van Horn, E.C.** "Programming Semantics for Multiprogrammed Computations." *CACM* 9(3), 1966
