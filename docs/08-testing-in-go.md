# 08 — Testing в Go

## Встроенный пакет `testing`

Go не требует внешнего test runner. Всё в стандартной библиотеке.

**Конвенции:**
- Тестовые файлы: `*_test.go` (исключаются из обычных билдов)
- Тестовые функции: начинаются с `Test`, принимают `*testing.T`
- Живут рядом с тестируемым кодом

```go
func TestNewSession(t *testing.T) {
    s := core.NewSession("ws://example.com")
    if s.State != core.StateCreated {
        t.Errorf("state = %v, want StateCreated", s.State)
    }
    if s.Target != "ws://example.com" {
        t.Errorf("target = %q, want %q", s.Target, "ws://example.com")
    }
}
```

**Нет встроенных assertions.** Go намеренно не имеет `assert.Equal()`. Тесты — обычный Go-код с `if` + `t.Errorf`. Философия: не нужен DSL поверх языка.

### Запуск тестов

```bash
go test ./...              # все пакеты
go test -v ./internal/core # verbose, конкретный пакет
go test -run TestNewSession # фильтр по имени
go test -count=1 ./...     # отключить кэш тестов
go test -race ./...        # race detector (обязательно для concurrent-кода!)
```

### White-box vs Black-box

```go
package core       // white-box: доступ к неэкспортированным полям (frames)
package core_test  // black-box: только через публичный API
```

---

## Table-Driven Tests — главный паттерн

Вместо отдельной функции на каждый кейс — тесты как данные:

```go
func TestDirection(t *testing.T) {
    tests := []struct {
        name      string
        direction core.Direction
        wantStr   string
    }{
        {"client to server", core.ClientToServer, "client_to_server"},
        {"server to client", core.ServerToClient, "server_to_client"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := tt.direction.String()
            if got != tt.wantStr {
                t.Errorf("Direction.String() = %q, want %q", got, tt.wantStr)
            }
        })
    }
}
```

**Почему:**
- Добавить кейс = добавить строку в slice
- Все кейсы видны в одном месте — легко ревьюить coverage gaps
- Каждый кейс имеет имя → структурированный вывод: `--- FAIL: TestDirection/client_to_server`
- Логика assertion написана один раз

### Типичные поля struct

```go
tests := []struct {
    name    string       // имя субтеста
    input   SomeInput    // входные данные
    want    SomeOutput   // ожидаемый результат
    wantErr bool         // ожидается ли ошибка
}{}
```

---

## Subtests с `t.Run()`

```go
func TestSessionState(t *testing.T) {
    t.Run("initial state", func(t *testing.T) {
        s := core.NewSession("ws://example.com")
        if s.State != core.StateCreated {
            t.Errorf("want StateCreated, got %v", s.State)
        }
    })

    t.Run("closed session has close code", func(t *testing.T) {
        // ...
    })
}
```

**Преимущества:**
1. Выборочный запуск: `go test -run TestSessionState/initial_state`
2. Независимые отказы: один субтест падает — остальные бегут
3. Каждый `t.Run` — своя настройка и teardown
4. Иерархический вывод в verbose mode

---

## `t.Parallel()` — конкурентные тесты

```go
func TestParallel(t *testing.T) {
    tests := []struct {
        name   string
        target string
    }{
        {"localhost", "ws://localhost:3000"},
        {"remote", "ws://remote.example.com"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel() // этот субтест бежит конкурентно

            s := core.NewSession(tt.target)
            if s.Target != tt.target {
                t.Errorf("got %q, want %q", s.Target, tt.target)
            }
        })
    }
}
```

### Gotcha: loop variable (Go < 1.22)

```go
// СЛОМАНО в Go < 1.22:
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        t.Parallel()
        // tt мог сдвинуться на последний элемент к моменту запуска!
    })
}

// FIX:
for _, tt := range tests {
    tt := tt // shadowing — копия на каждую итерацию
    t.Run(tt.name, func(t *testing.T) {
        t.Parallel()
        // OK
    })
}
```

**Go 1.22+:** loop variable scoped per-iteration когда `go` directive >= 1.22. `tt := tt` больше не нужен.

### Gotcha: context timeout

Всегда вызывай `t.Parallel()` **до** `context.WithTimeout()`. `t.Parallel()` паузит тест пока родительский не закончится — timeout может истечь до старта.

---

## `httptest` — тестирование без реального сервера

### `httptest.NewRecorder()` — unit test хендлера

```go
func TestHealthHandler(t *testing.T) {
    req := httptest.NewRequest("GET", "/health", nil)
    w := httptest.NewRecorder()

    HealthHandler(w, req)

    if w.Code != http.StatusOK {
        t.Errorf("status = %d, want 200", w.Code)
    }
}
```

Нет сети — прямой вызов хендлера.

### `httptest.NewServer()` — integration test с реальным TCP

```go
func TestWebSocketEcho(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(wsEchoHandler))
    defer srv.Close()

    // http:// → ws://
    wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

    conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil {
        t.Fatalf("dial: %v", err)
    }
    defer conn.Close()

    msg := []byte("hello")
    if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
        t.Fatalf("write: %v", err)
    }

    _, got, err := conn.ReadMessage()
    if err != nil {
        t.Fatalf("read: %v", err)
    }
    if string(got) != "hello" {
        t.Errorf("got %q, want %q", got, "hello")
    }
}
```

**Для WebSocket тестов обязателен `httptest.NewServer`** — нужен реальный TCP для HTTP Upgrade.

---

## `testdata/` и Golden Files

`testdata/` — специальная директория, игнорируется `go build`.

```
internal/storage/
    store.go
    store_test.go
    testdata/
        session.golden.json
        frames.golden.json
```

### Golden file pattern

```go
var update = flag.Bool("update", false, "update golden files")

func TestExportSession(t *testing.T) {
    session := createTestSession()
    got := ExportJSON(session)

    golden := "testdata/session.golden.json"
    if *update {
        os.WriteFile(golden, got, 0644)
        return
    }

    want, err := os.ReadFile(golden)
    if err != nil {
        t.Fatal(err)
    }

    if !bytes.Equal(got, want) {
        t.Errorf("output mismatch:\ngot:\n%s\nwant:\n%s", got, want)
    }
}
```

Запуск: `go test -update` для регенерации golden files. Никогда в CI.

---

## Тестовые дубли без фреймворков

Go-интерфейсы делают моки тривиальными:

```go
// Fake (простая in-memory реализация):
type fakeStore struct {
    sessions map[uuid.UUID]*core.Session
}

func (f *fakeStore) GetSession(_ context.Context, id uuid.UUID) (*core.Session, error) {
    s, ok := f.sessions[id]
    if !ok {
        return nil, storage.ErrNotFound
    }
    return s, nil
}

// Stub (всегда возвращает фиксированное значение):
type stubStore struct {
    err error
}

func (s *stubStore) GetSession(context.Context, uuid.UUID) (*core.Session, error) {
    return nil, s.err
}

// Function-based (максимальная гибкость):
type funcStore struct {
    getSessionFunc func(ctx context.Context, id uuid.UUID) (*core.Session, error)
}

func (f *funcStore) GetSession(ctx context.Context, id uuid.UUID) (*core.Session, error) {
    return f.getSessionFunc(ctx, id)
}
```

**Почему Go-сообщество избегает тяжёлых mock-библиотек:**
1. Интерфейсы маленькие — 1-2 метода, fake пишется за 5 строк
2. Без reflection — plain Go, легко читать и дебажить
3. Compile-time safety — если интерфейс изменился, fake не скомпилируется
4. Нет внешних зависимостей

---

## `t.Helper()` — правильные номера строк

```go
// БЕЗ t.Helper():
func assertEqual(t *testing.T, got, want int) {
    if got != want {
        t.Errorf("got %d, want %d", got, want) // ошибка указывает СЮДА
    }
}

// С t.Helper():
func assertEqual(t *testing.T, got, want int) {
    t.Helper() // помечаем как helper
    if got != want {
        t.Errorf("got %d, want %d", got, want) // ошибка указывает на ВЫЗОВ
    }
}
```

Вызывать `t.Helper()` первой строкой в хелпер-функциях. Принимай `testing.TB` если хелпер для тестов и бенчмарков.

---

## Benchmarks (`testing.B`)

```go
func BenchmarkNewSession(b *testing.B) {
    for i := 0; i < b.N; i++ {
        core.NewSession("ws://example.com")
    }
}
```

Запуск:

```bash
go test -bench=. -benchmem
```

Вывод:

```
BenchmarkNewSession-8   5000000   300 ns/op   256 B/op   2 allocs/op
```

### Исключение setup из замеров

```go
func BenchmarkProcessFrames(b *testing.B) {
    frames := generateTestFrames(10000)
    b.ResetTimer() // setup не считается

    for i := 0; i < b.N; i++ {
        ProcessFrames(frames)
    }
}
```

### Сравнение бенчмарков

```bash
go test -bench=. -count=10 > old.txt
# после изменений:
go test -bench=. -count=10 > new.txt
benchstat old.txt new.txt
```

---

## Сравнение с pytest

| Аспект | Go `testing` | Python `pytest` |
|--------|-------------|----------------|
| Установка | Встроен в язык | `pip install pytest` |
| Assertions | Нет встроенных (`if` + `t.Errorf`) | `assert x == y` с авто-diff |
| Fixtures | Helper-функции, `t.Cleanup` | `@pytest.fixture` с DI |
| Параметризация | Table-driven tests | `@pytest.mark.parametrize` |
| Параллелизм | `t.Parallel()` встроен | `pytest-xdist` плагин |
| Mocking | Интерфейсы + ручные fakes | `unittest.mock`, monkeypatch |
| Benchmarks | `testing.B` встроен | `pytest-benchmark` плагин |
| Coverage | `go test -cover` встроен | `pytest-cov` плагин |
| Race detection | `go test -race` встроен | Нет |
| Fuzzing | `testing.F` встроен (Go 1.18+) | `hypothesis` |
| Философия | "Тесты — обычный Go-код" | "Максимально мощно и удобно" |

**Главное отличие:** Go минимализм vs pytest-магия. В Go test setup — явные вызовы функций. В pytest — fixture injection (тест объявляет параметр `db`, pytest магически его предоставляет).

---

## Теоретические истоки

### Table-Driven Tests — Data-Driven Testing

Table-driven tests — идиоматический паттерн, не фича фреймворка. Истоки:
- **Data-driven testing** (QA community, 1990-е): тестовые данные отделены от логики, driver итерирует
- **xUnit parameterized tests** (JUnit `@Parameterized`, pytest `@pytest.mark.parametrize`): формализация в unit test frameworks
- **Kent Beck.** *Test Driven Development: By Example.* Addison-Wesley, 2002 — SUnit (Smalltalk, 1989) → xUnit architecture

Go использует **anonymous struct slices** — нет декораторов, аннотаций, фреймворков. Просто данные в коде.

### Property-Based Testing (QuickCheck, 2000)

Альтернатива table-driven: вместо конкретных input/output пар — задаёшь **свойства**, фреймворк генерирует random inputs и **shrinks** counterexample до минимума.

**Claessen, K. & Hughes, J.** "QuickCheck: A Lightweight Tool for Random Testing of Haskell Programs." *ICFP*, 2000.

В Go: `testing/quick` (stdlib), `gopter`, `rapid` (third-party).

### Тестовые дубли и ISP

Go подход "интерфейсы вместо mock-фреймворков" — прямое следствие ISP (Interface Segregation Principle). Маленький интерфейс = тривиальный fake. Это **design for testability** — решение принимается при проектировании API, а не при написании тестов.

Сравни с Python `unittest.mock.patch()` — может замокать что угодно через monkey-patching, но это runtime magic, а не compile-time guarantees.

---

## Что читать дальше

### Тестирование в Go
- **Go Wiki.** [TableDrivenTests](https://go.dev/wiki/TableDrivenTests) — каноническое описание
- **Cheney, Dave.** ["Prefer table driven tests."](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests) 2019
- **quii.** [Learn Go with Tests](https://quii.gitbook.io/learn-go-with-tests/) — TDD tutorial

### Теория тестирования
- **Beck, K.** *Test Driven Development: By Example.* Addison-Wesley, 2002
- **Claessen, K. & Hughes, J.** "QuickCheck..." *ICFP*, 2000
- **Karp, Samuel.** ["Flexible Test Doubles in Go."](https://samuel.karp.dev/blog/2023/02/flexible-test-doubles-in-go/) 2023
