# 10 — Философия тестирования: уроки SQLite

## Почему SQLite — эталон

SQLite — самое развёрнутое ПО в мире (триллионы инсталляций: каждый телефон, браузер, ОС). И одно из самых тестируемых:

| Метрика | Значение |
|---------|----------|
| Исходный код | ~155,800 строк C |
| Тестовый код | ~92,053,100 строк |
| **Соотношение** | **590:1 (тесты : код)** |
| Тестовых кейсов (TCL) | 51,445 |
| Тестовых кейсов (TH3) | 50,362 |
| Инстансов тестов (full run) | ~2.4 миллиона |
| Soak-тест перед релизом | ~248.5 миллионов |
| SQL Logic Test | 7.2 миллиона запросов (1.12 GB данных) |
| assert() в коде | 6,754 штук |
| Фаззинг (dbsqlfuzz) | ~1 миллиард мутаций в день |

При этом SQLite не использует фреймворков, CI/CD-конвейеров и автоматических пайплайнов для финального тестирования — pre-release checklist из ~200 пунктов выполняется разработчиками вручную.

---

## Четыре независимых тестовых harness'а

SQLite не полагается на один подход. Четыре **независимо написанных** тестовых системы ловят разные классы багов:

```
┌─────────────────────────────────────────────────────────────────┐
│                     SQLite Test Strategy                        │
├──────────────┬──────────────┬───────────────┬──────────────────┤
│   TCL Tests  │     TH3      │  SQL Logic    │   dbsqlfuzz      │
│              │              │   Test        │                  │
│ 51K кейсов   │ 50K кейсов   │ 7.2M запросов │ 1B мутаций/день  │
│ Скриптовые   │ C-код        │ Сравнение с   │ libFuzzer        │
│ Основные     │ 100% branch  │ PostgreSQL,   │ Мутирует и SQL,  │
│ для разрабки │ 100% MC/DC   │ MySQL, Oracle │ и .db файлы      │
└──────────────┴──────────────┴───────────────┴──────────────────┘
```

**Ключевая идея:** каждый harness находит баги, которые другие пропускают. TCL-тесты ловят логические ошибки. TH3 гарантирует coverage. SQL Logic Test находит семантические расхождения с другими БД. dbsqlfuzz обнаруживает crashes и undefined behavior на входах, которые человек никогда бы не написал.

### Маппинг на Go-проект

| SQLite Harness | Go-эквивалент | Для WebSocket-прокси |
|----------------|---------------|---------------------|
| TCL Tests | `go test` (table-driven) | Unit-тесты core, storage, CLI |
| TH3 (coverage) | `go test -cover -coverprofile` | Branch coverage всех пакетов |
| SQL Logic Test | Integration tests с `httptest` | Сравнение поведения с эталонной реализацией |
| dbsqlfuzz | `testing.F` (Go 1.18+ fuzz) | Фаззинг frame parser, CLI args, SQL queries |

---

## Уровень 1: Coverage — необходимо, но недостаточно

### Иерархия coverage

```
Statement Coverage      ← "строка исполнена" (самое слабое)
    ↓
Branch Coverage         ← "каждый if проверен в обе стороны" (SQLite: 100%)
    ↓
MC/DC Coverage          ← "каждое условие независимо влияет на результат"
    ↓                       (SQLite: 100%, стандарт авиации DO-178B)
Mutation Testing        ← "тест ловит каждое возможное изменение кода"
```

**Statement coverage обманчив:**

```go
func categorize(frame *Frame) string {
    if frame.Opcode == 1 || frame.Opcode == 2 {
        return "data"
    }
    return "control"
}
```

Один тест с `Opcode == 1` даёт 100% statement coverage, но не тестирует `Opcode == 2` и не проверяет граничные значения (0, 8, 9, 10, 15).

**Branch coverage** требует оба исхода каждого `if`. Но даже этого мало:

```go
if a && b {  // branch coverage: true/false достаточно
             // MC/DC: нужно доказать что a и b НЕЗАВИСИМО влияют
```

Для `a && b` MC/DC требует минимум 3 теста: `(T,T)→true`, `(F,T)→false`, `(T,F)→false`.

### Coverage в Go

```bash
# Statement coverage (дефолт)
go test -cover ./...

# Branch coverage profile
go test -coverprofile=coverage.out -covermode=atomic ./...
go tool cover -html=coverage.out    # визуализация

# С race detector (обязательно для concurrent кода)
go test -race -cover ./...
```

Go `go test -cover` измеряет **statement coverage**. Для branch coverage нужен `-covermode=atomic` + анализ профиля. Полноценного MC/DC в Go-экосистеме нет, но комбинация table-driven тестов + fuzz + mutation testing приближает.

### Mutation testing в Go

SQLite использует скрипт, который заменяет каждый branch на unconditional jump / no-op и проверяет что тесты ловят замену. В Go:

```bash
# gremlins — mutation testing tool для Go
go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
gremlins unleash ./...
```

Gremlins мутирует код (заменяет `>` на `>=`, `&&` на `||`, убирает строки) и проверяет что хотя бы один тест падает на каждой мутации. "Выжившие" мутанты = пробелы в тестах.

---

## Уровень 2: Anomaly Testing — тестирование когда мир ломается

**Это главный урок SQLite.** Большинство разработчиков тестируют только happy path. SQLite тестирует каждый возможный failure mode.

### OOM Testing в SQLite

SQLite подменяет `malloc()` через `sqlite3_config(SQLITE_CONFIG_MALLOC, ...)`. Тест запускается в цикле:

```
Итерация 1: malloc() падает на 1-й аллокации
Итерация 2: malloc() падает на 2-й аллокации
...
Итерация N: malloc() падает на N-й аллокации (N-я аллокация — последняя)
```

Каждая итерация проверяет: SQLite вернул корректную ошибку, не потерял данные, не утёк memory.

Более того: **каждый цикл прогоняется дважды** — с однократным отказом malloc и с постоянным отказом после N-й аллокации. Это ловит баги в путях восстановления.

### I/O Error Testing в SQLite

SQLite использует **Virtual File System (VFS)** — абстракцию над файловой системой. Для тестов подставляется VFS, который:
- Симулирует ошибку чтения после N-й операции
- Симулирует ошибку записи после N-й операции
- Симулирует disk full
- Симулирует corrupted read

После каждого теста — `PRAGMA integrity_check` для верификации целостности БД.

### Crash Testing в SQLite

**TCL-подход:** дочерний процесс выполняет SQL-операции и случайно крашится (kill -9) в середине записи. Специальный VFS **переупорядочивает и повреждает** незакоммиченные записи, симулируя реальное поведение файловой системы при потере питания. Родительский процесс проверяет: транзакция либо завершена полностью, либо полностью откачена.

**TH3-подход:** in-memory VFS делает snapshot файловой системы после каждой N-й I/O-операции. Цикл прогоняет тест, на каждой итерации откатывая БД к snapshot'у с характерными повреждениями от power loss.

### Compound Failures

Самое хардкорное: **тестирование I/O ошибки во время восстановления после предыдущего краша**. Два уровня отказа одновременно.

---

## Маппинг аномалий на WebSocket-прокси

### Fault Matrix для проекта

| Компонент | Аномалия | Как симулировать в Go | Критичность |
|-----------|----------|----------------------|-------------|
| **Frame parser** | Malformed frame (неверный opcode, payload > 2^63) | Fuzz-тест, ручные bad frames | Высокая — crash или panic |
| **Frame parser** | Truncated frame (FIN=0, потом connection drop) | Write partial data, close conn | Высокая |
| **Relay** | Client disconnects mid-message | `conn.Close()` в горутине | Высокая — goroutine leak |
| **Relay** | Server не отвечает (hang) | `httptest.Server` с `time.Sleep(∞)` | Высокая — timeout needed |
| **Relay** | Slow consumer (TUI зависает) | Buffered channel, не читать | Средняя — backpressure |
| **Storage** | SQLite write failure (disk full) | Custom VFS / tmpfs с лимитом | Высокая — потеря фреймов |
| **Storage** | Concurrent write contention | `t.Parallel()` + shared DB | Средняя — busy timeout |
| **Storage** | DB file corrupted | Записать мусор в .db файл | Средняя — graceful error |
| **CLI** | Invalid arguments | Table-driven tests | Низкая |
| **Pub/Sub** | Subscriber panic | `recover()` в горутине | Высокая — bus не должен упасть |
| **Close handshake** | Peer не отвечает на Close frame | Не отвечать в тесте, проверить timeout | Высокая |
| **Masking** | Frame с MASK=1 от сервера (нарушение RFC) | Ручной malformed frame | Средняя |

### Fault Injection в Go: паттерн через интерфейсы

SQLite использует VFS для подмены файловой системы. В Go — **интерфейсы**:

```go
// Абстракция для внедрения сбоев
type FrameWriter interface {
    WriteFrame(ctx context.Context, f *core.Frame) error
}

// Продакшен-реализация
type SQLiteFrameWriter struct {
    store *storage.Store
}

func (w *SQLiteFrameWriter) WriteFrame(ctx context.Context, f *core.Frame) error {
    return w.store.InsertFrame(ctx, f)
}

// Fault-injection реализация для тестов
type faultyFrameWriter struct {
    real       FrameWriter
    failAfter  int      // падать после N-й записи
    failErr    error    // какую ошибку возвращать
    callCount  int
}

func (w *faultyFrameWriter) WriteFrame(ctx context.Context, f *core.Frame) error {
    w.callCount++
    if w.callCount >= w.failAfter {
        return w.failErr
    }
    return w.real.WriteFrame(ctx, f)
}
```

Это прямой аналог SQLite VFS injection: тестовая обёртка считает вызовы и возвращает ошибку на N-м вызове.

### Loop-style fault injection (как SQLite OOM-тесты)

```go
func TestRelayHandlesWriteFailure(t *testing.T) {
    // Для каждой возможной точки отказа...
    for failAt := 1; failAt <= 20; failAt++ {
        t.Run(fmt.Sprintf("fail_at_write_%d", failAt), func(t *testing.T) {
            writer := &faultyFrameWriter{
                real:      &memoryWriter{},
                failAfter: failAt,
                failErr:   errors.New("disk full"),
            }

            err := runRelay(ctx, client, server, writer)

            // Проверяем: relay корректно завершился с ошибкой,
            // не паникнул, не утёк горутинами
            if err == nil {
                t.Fatal("expected error from faulty writer")
            }

            // Все фреймы до точки отказа записаны
            if writer.callCount < failAt-1 {
                t.Errorf("expected at least %d writes, got %d",
                    failAt-1, writer.callCount)
            }
        })
    }
}
```

Каждая итерация — отказ в новой точке. Это **прямая калька** с SQLite OOM-тестирования.

---

## Уровень 3: Fuzz Testing

### SQLite и фаззинг

SQLite использует четыре фаззера:

1. **AFL** (American Fuzzy Lop) — profile-guided, сохраняет inputs вызывающие новые code paths
2. **OSS-Fuzz** (Google) — непрерывный автоматический фаззинг на инфраструктуре Google
3. **dbsqlfuzz** — мутирует **одновременно** SQL и .db файл (структурно-осведомлённый мутатор)
4. **jfuzz** (с янв. 2024) — генерирует corrupt JSONB blobs для JSON-функций

**Ключевой инсайт SQLite:** *"Fuzz testing and 100% MC/DC testing are in tension."* MC/DC не поощряет defensive code с недостижимыми ветками. Но без такого кода фаззер находит больше проблем. SQLite решает это так: **основные CPU-циклы тратятся на фаззинг**, а MC/DC поддерживается параллельно.

### Go native fuzz testing (testing.F)

Go 1.18+ имеет встроенный фаззинг. Для WebSocket-прокси это критично — frame parser принимает **произвольные байты из сети**.

```go
// Фаззинг WebSocket frame parser
func FuzzParseFrame(f *testing.F) {
    // Seed corpus — корректные фреймы
    f.Add([]byte{0x81, 0x05, 0x48, 0x65, 0x6c, 0x6c, 0x6f}) // "Hello" text
    f.Add([]byte{0x82, 0x02, 0xAB, 0xCD})                     // binary, 2 bytes
    f.Add([]byte{0x88, 0x02, 0x03, 0xE8})                     // close, code 1000
    f.Add([]byte{0x89, 0x00})                                   // ping, empty
    f.Add([]byte{0x8A, 0x00})                                   // pong, empty

    // Фаззер мутирует seed'ы, ищет crashes и panics
    f.Fuzz(func(t *testing.T, data []byte) {
        frame, err := ParseFrame(data)
        if err != nil {
            return // ошибка парсинга — OK, главное не паника
        }

        // Property: если парсинг успешен, фрейм должен быть валидным
        if frame.Opcode > 15 {
            t.Errorf("opcode %d out of range [0,15]", frame.Opcode)
        }

        // Property: roundtrip — сериализация и обратный парсинг
        // должны дать тот же фрейм
        serialized := frame.Serialize()
        reparsed, err := ParseFrame(serialized)
        if err != nil {
            t.Fatalf("roundtrip failed: serialize then parse: %v", err)
        }
        if reparsed.Opcode != frame.Opcode {
            t.Errorf("roundtrip opcode: got %d, want %d",
                reparsed.Opcode, frame.Opcode)
        }
    })
}
```

```bash
# Запуск фаззинга (бежит бесконечно, ищет crashes)
go test -fuzz=FuzzParseFrame -fuzztime=5m ./internal/core/

# Найденные crashes сохраняются в testdata/fuzz/FuzzParseFrame/
# и автоматически включаются в будущие go test ./...
```

### Что фаззить в проекте

| Компонент | Fuzz target | Свойства для проверки |
|-----------|-------------|----------------------|
| Frame parser | `FuzzParseFrame([]byte)` | Не паникует, roundtrip, опкод в [0,15] |
| Close code parser | `FuzzParseCloseCode([]byte)` | Код в валидных диапазонах или ошибка |
| Masking/unmasking | `FuzzMaskUnmask(key, payload)` | `unmask(mask(x)) == x` всегда |
| UTF-8 validation (text frames) | `FuzzValidateUTF8([]byte)` | Согласованность с `utf8.Valid` |
| CLI argument parsing | `FuzzParseArgs(string)` | Не паникует, валидная ошибка |
| SQL query builder | `FuzzBuildQuery(filters)` | SQL-инъекции невозможны, запрос синтаксически корректен |

### Masking roundtrip — идеальный кандидат для fuzz

```go
func FuzzMaskRoundtrip(f *testing.F) {
    f.Add([]byte{0xAB, 0xCD, 0xEF, 0x01}, []byte("Hello, WebSocket!"))
    f.Add([]byte{0x00, 0x00, 0x00, 0x00}, []byte{})
    f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF}, []byte{0x00})

    f.Fuzz(func(t *testing.T, key []byte, payload []byte) {
        if len(key) != 4 {
            t.Skip("masking key must be 4 bytes")
        }

        masked := applyMask(key, payload)
        unmasked := applyMask(key, masked)

        if !bytes.Equal(unmasked, payload) {
            t.Errorf("mask roundtrip failed:\n  key=%x\n  payload=%x\n  got=%x",
                key, payload, unmasked)
        }
    })
}
```

XOR — собственная инверсия, поэтому `mask(mask(x)) == x` **всегда**. Это математическое свойство, и фаззер должен не найти контрпример.

---

## Уровень 4: Assertions — исполняемая документация

### Подход SQLite: 6,754 assert()

SQLite содержит тысячи `assert()` — проверок инвариантов, которые:
- **Включены** в debug-билдах (замедление ~3x)
- **Выключены** в release-билдах (zero cost)
- Проверяют предусловия, постусловия, инварианты циклов

Это **не** тесты. Это **контракты**, вшитые в код, которые документируют ожидания автора и ловят нарушения в runtime.

### Go-аналог: build tags

Go не имеет встроенных assertions, но build tags дают тот же эффект:

```go
//go:build debug

package core

// debugAssert panics with msg if cond is false.
// Only compiled in debug builds: go test -tags debug
func debugAssert(cond bool, msg string) {
    if !cond {
        panic("assertion failed: " + msg)
    }
}
```

```go
//go:build !debug

package core

// debugAssert is a no-op in release builds.
func debugAssert(_ bool, _ string) {}
```

Использование в коде:

```go
func (s *Session) AddFrame(f *Frame) {
    debugAssert(s.State == StateActive,
        "AddFrame called on non-active session")
    debugAssert(f.Timestamp.After(s.CreatedAt),
        "frame timestamp before session creation")

    s.frames = append(s.frames, f)
}
```

```bash
# Тесты с assertions (медленнее, ловят инварианты)
go test -tags debug -race ./...

# Прод-билд (assertions = no-op, zero cost)
go build ./...
```

### SQLite ALWAYS/NEVER макросы → Go-аналог

SQLite использует специальные макросы для условий, которые **теоретически** всегда true/false:

```c
/* SQLite C */
if (NEVER(pExpr == NULL)) return;  // pExpr никогда не должен быть NULL
if (ALWAYS(n > 0)) { ... }        // n всегда должен быть > 0
```

В тестах NEVER/ALWAYS = assert. В coverage-билдах = константы (чтобы не генерировать unreachable branch).

Go-аналог:

```go
func processFrame(f *Frame) {
    // Это условие никогда не должно быть true в корректной программе.
    // Но мы защищаемся и проверяем в тестах.
    if f == nil {
        debugAssert(false, "processFrame called with nil frame")
        return // defensive: в прод-билде не крашимся
    }
    // ...
}
```

---

## Уровень 5: Property-Based Testing

### От примеров к свойствам

Table-driven тесты проверяют конкретные пары `input → expected output`. Property-based тесты проверяют **инварианты** на **случайных** входах.

| Table-driven | Property-based |
|-------------|---------------|
| "Hello" → текстовый фрейм с opcode 1 | Для любого payload: `parse(serialize(frame)) == frame` |
| Фрейм 125 байт → payload length в 1 байт | Для любого размера: длина правильно кодируется |
| Конкретный close code 1000 → "Normal Closure" | Для любого code ∈ [1000,4999]: парсится без ошибки |

### `rapid` — Go библиотека для property-based testing

```go
import "pgregory.net/rapid"

func TestFrameRoundtrip(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        // Генерируем случайный фрейм
        opcode := rapid.IntRange(0, 15).Draw(t, "opcode")
        payload := rapid.SliceOf(rapid.Byte()).Draw(t, "payload")
        fin := rapid.Bool().Draw(t, "fin")

        frame := &Frame{
            FIN:     fin,
            Opcode:  byte(opcode),
            Payload: payload,
        }

        // Свойство: roundtrip
        serialized := frame.Serialize()
        parsed, err := ParseFrame(serialized)
        if err != nil {
            t.Fatalf("roundtrip failed: %v", err)
        }

        if parsed.Opcode != frame.Opcode {
            t.Fatalf("opcode mismatch: %d != %d", parsed.Opcode, frame.Opcode)
        }
        if !bytes.Equal(parsed.Payload, frame.Payload) {
            t.Fatal("payload mismatch")
        }
    })
}
```

`rapid` при провале автоматически **shrinks** ввод до минимального контрпримера — как QuickCheck.

### Какие свойства проверять в проекте

| Компонент | Свойство | Формулировка |
|-----------|----------|-------------|
| Frame parser | Roundtrip | `parse(serialize(f)) ≡ f` |
| Masking | Self-inverse | `mask(key, mask(key, data)) ≡ data` |
| Session | Monotonic timestamps | `∀ i: frames[i].Timestamp ≤ frames[i+1].Timestamp` |
| Session state | Valid transitions | `transition(state, event)` всегда в множестве допустимых |
| Pub/Sub Bus | No message loss (buffered) | Если буфер не полон, все сообщения доставлены |
| Storage | Insert then get | `get(insert(frame).ID) ≡ frame` |
| Payload length encoding | Correct range | `len ≤ 125 → 1 byte`, `len ≤ 65535 → 3 bytes`, иначе → 9 bytes |
| Close code | Valid ranges | Код ∈ {1000-1003, 1007-1011, 3000-4999} или ошибка |

---

## Уровень 6: Regression Testing

### Философия SQLite

> *"Whenever a bug is reported against SQLite, that bug is not considered fixed until new test cases that would exhibit the bug have been added to either the TCL or TH3 test suites."*

Каждый баг получает тест **до** исправления. Этот процесс за десятилетия накопил тысячи regression-тестов.

### Реализация в Go

Простой паттерн: директория `testdata/regressions/`:

```
internal/core/
    testdata/
        regressions/
            issue_042_truncated_close_frame.bin
            issue_057_zero_length_text.bin
            issue_063_oversized_payload_length.bin
```

```go
func TestRegressions(t *testing.T) {
    entries, err := os.ReadDir("testdata/regressions")
    if err != nil {
        t.Fatal(err)
    }

    for _, entry := range entries {
        t.Run(entry.Name(), func(t *testing.T) {
            data, err := os.ReadFile(filepath.Join("testdata/regressions", entry.Name()))
            if err != nil {
                t.Fatal(err)
            }

            // Каждый файл — input, вызвавший баг.
            // Парсер не должен паниковать.
            _, parseErr := ParseFrame(data)

            // Для malformed inputs — ожидаем ошибку, не панику
            if parseErr == nil {
                t.Log("parsed successfully (may be valid frame)")
            }
        })
    }
}
```

Фаззер автоматически сохраняет crashes в `testdata/fuzz/` — это тоже regression-тесты. `go test` прогоняет их при каждом запуске.

---

## Уровень 7: Boundary Value Testing

### Подход SQLite: 1,184 testcase() макроса

SQLite использует `testcase()` макрос для **явной маркировки граничных значений**. Каждый `testcase()` — утверждение: "этот тест попал на конкретную сторону границы".

### Граничные значения WebSocket

| Граница | Значения для тестирования | Почему важно |
|---------|--------------------------|-------------|
| Payload length encoding | 0, 1, 125, **126**, 127, 65535, **65536**, 2^63-1 | 126 = переход на 16-bit encoding, 65536 = переход на 64-bit |
| Control frame size | 0, 1, 124, **125**, 126 | Control frames max 125 bytes payload |
| Close code | 999, **1000**, 1011, 1012, 2999, **3000**, 4999, **5000** | Валидные диапазоны: 1000-1011, 3000-4999 |
| Masking key | `{0,0,0,0}`, `{FF,FF,FF,FF}`, random | Edge cases XOR |
| Fragment count | 1 (не фрагментирован), 2, 1000 | Первый/последний + continuation |
| UTF-8 в text frame | Empty string, ASCII, multi-byte, max codepoint, **invalid** | RFC требует валидный UTF-8 для opcode 1 |
| Concurrent sessions | 0, 1, 2, 100, `GOMAXPROCS * 100` | Concurrency edge cases |

```go
func TestPayloadLengthEncoding(t *testing.T) {
    // Граничные значения для variable-length encoding
    boundaries := []struct {
        name     string
        length   int
        wantBytes int // сколько байт на encoding
    }{
        {"zero",                 0,         1},
        {"one",                  1,         1},
        {"max_7bit",             125,       1},  // ← граница
        {"min_16bit",            126,       3},  // ← переход
        {"mid_16bit",            1000,      3},
        {"max_16bit",            65535,     3},  // ← граница
        {"min_64bit",            65536,     9},  // ← переход
        {"large",                1_000_000, 9},
    }

    for _, b := range boundaries {
        t.Run(b.name, func(t *testing.T) {
            encoded := encodePayloadLength(b.length)
            if len(encoded) != b.wantBytes {
                t.Errorf("length %d: encoded to %d bytes, want %d",
                    b.length, len(encoded), b.wantBytes)
            }

            decoded, err := decodePayloadLength(encoded)
            if err != nil {
                t.Fatalf("decode failed: %v", err)
            }
            if decoded != b.length {
                t.Errorf("roundtrip: got %d, want %d", decoded, b.length)
            }
        })
    }
}
```

---

## Уровень 8: Resource Leak Detection

### SQLite: автоматическое отслеживание

SQLite **на каждом тесте** автоматически проверяет:
- Нет утечек памяти (все malloc → free)
- Нет утечек file descriptors
- Нет утечек мьютексов
- Нет утечек тредов

Без специальной конфигурации — встроено в тестовый harness.

### Go-аналог: goroutine leak detection

Для Go самый критичный ресурс — горутины. Если relay-горутина не завершается после закрытия соединения — goroutine leak.

```go
import "go.uber.org/goleak"

func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m)
}
```

`goleak` проверяет после **каждого** теста: количество горутин вернулось к исходному. Любая утечённая горутина = fail.

Или без зависимости:

```go
func assertNoGoroutineLeak(t *testing.T) {
    t.Helper()

    before := runtime.NumGoroutine()
    t.Cleanup(func() {
        // Даём горутинам время завершиться
        deadline := time.Now().Add(2 * time.Second)
        for time.Now().Before(deadline) {
            after := runtime.NumGoroutine()
            if after <= before {
                return
            }
            runtime.Gosched()
            time.Sleep(10 * time.Millisecond)
        }
        t.Errorf("goroutine leak: before=%d, after=%d",
            before, runtime.NumGoroutine())
    })
}

func TestRelayNoLeak(t *testing.T) {
    assertNoGoroutineLeak(t)
    // ... тест relay ...
}
```

### Другие ресурсы для проверки

```go
func TestStorageNoFileDescriptorLeak(t *testing.T) {
    // Открываем и закрываем store много раз
    for i := 0; i < 100; i++ {
        store, err := storage.Open(":memory:")
        if err != nil {
            t.Fatal(err)
        }
        store.Close()
    }

    // Если file descriptors утекают, 100 итераций
    // исчерпают ulimit и Open вернёт ошибку
}
```

---

## Уровень 9: Chaos / Integration Testing

### WebSocket-специфичные сценарии

```go
func TestRelayHandlesAbruptDisconnect(t *testing.T) {
    // Поднимаем echo-сервер
    upstream := httptest.NewServer(wsEchoHandler())
    defer upstream.Close()

    // Поднимаем прокси
    proxy := httptest.NewServer(proxyHandler(upstream.URL))
    defer proxy.Close()

    // Подключаемся через прокси
    wsURL := "ws" + strings.TrimPrefix(proxy.URL, "http")
    conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil {
        t.Fatal(err)
    }

    // Отправляем фрейм
    conn.WriteMessage(websocket.TextMessage, []byte("hello"))

    // РЕЗКО закрываем TCP (без close handshake)
    conn.UnderlyingConn().Close()

    // Даём прокси обработать
    time.Sleep(100 * time.Millisecond)

    // Проверяем: прокси не паникнул, сессия в состоянии Closed/Error,
    // goroutine не утекла
}

func TestRelayHandlesSlowUpstream(t *testing.T) {
    // Сервер, который принимает соединение но не отвечает
    slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Upgrade, но не читаем и не пишем
        upgrader := websocket.Upgrader{}
        conn, _ := upgrader.Upgrade(w, r, nil)
        defer conn.Close()
        time.Sleep(10 * time.Minute) // "зависший" сервер
    }))
    defer slowServer.Close()

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    // Прокси должен завершить сессию по timeout
    err := runProxySession(ctx, slowServer.URL)
    if !errors.Is(err, context.DeadlineExceeded) {
        t.Errorf("expected deadline exceeded, got: %v", err)
    }
}
```

### Тестирование close handshake FSM

```go
func TestCloseHandshakeStateMachine(t *testing.T) {
    tests := []struct {
        name     string
        sequence []closeEvent
        wantState SessionState
    }{
        {
            name: "normal: client initiates",
            sequence: []closeEvent{
                {from: Client, code: 1000},  // client sends Close
                {from: Server, code: 1000},  // server responds Close
            },
            wantState: StateClosed,
        },
        {
            name: "server initiates",
            sequence: []closeEvent{
                {from: Server, code: 1001},  // Going Away
                {from: Client, code: 1001},
            },
            wantState: StateClosed,
        },
        {
            name: "no response to close",
            sequence: []closeEvent{
                {from: Client, code: 1000},
                // timeout — no response
            },
            wantState: StateError,
        },
        {
            name: "abnormal: connection drop",
            sequence: []closeEvent{
                {from: Client, code: 1006}, // pseudo-code: abnormal
            },
            wantState: StateError,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            session := NewSession("ws://example.com")
            session.State = StateActive

            for _, ev := range tt.sequence {
                session.HandleClose(ev)
            }

            if session.State != tt.wantState {
                t.Errorf("state = %v, want %v", session.State, tt.wantState)
            }
        })
    }
}
```

---

## Стратегия тестирования для проекта: пирамида

```
                    ╱╲
                   ╱  ╲          Manual / Exploratory
                  ╱ 🔍 ╲         (REPL, TUI — руками)
                 ╱──────╲
                ╱        ╲       Chaos / Integration
               ╱  🌪️ E2E  ╲      (real WS connections, fault injection,
              ╱────────────╲     httptest + gorilla/websocket)
             ╱              ╲
            ╱  🧪 Property   ╲   Property-based + Fuzz
           ╱   + Fuzz         ╲  (rapid, testing.F — random inputs)
          ╱────────────────────╲
         ╱                      ╲
        ╱   📋 Table-driven      ╲  Unit tests
       ╱    (core, storage, CLI)  ╲  (конкретные input/output, boundaries)
      ╱────────────────────────────╲
     ╱                              ╲
    ╱  🏃 go test -race              ╲  Race detector
   ╱   (каждый запуск, всегда)        ╲  (Go-уникальная суперсила)
  ╱────────────────────────────────────╲
```

### Checklist перед коммитом (аналог SQLite "veryquick")

```bash
# Быстрый набор — ~секунды, ловит большинство ошибок
go vet ./...
go test -race -count=1 ./...
golangci-lint run
```

### Checklist перед релизом (аналог SQLite full test + soak)

```bash
# Полный набор — минуты/часы
go test -race -count=1 ./...
go test -tags debug -race ./...         # с assertions
go test -fuzz=. -fuzztime=10m ./...     # фаззинг
go test -bench=. -benchmem ./...        # производительность
gremlins unleash ./...                  # mutation testing
```

---

## Defensive Programming: уроки из 6,754 assertions

### SQLite: каждый malloc проверен, каждый pointer dereferenced safely

SQLite рассматривает каждый вход как потенциально враждебный — не только SQL-запросы, но и сам .db файл. Любой байт .db файла может быть модифицирован злоумышленником.

### Для прокси: каждый байт из сети — враждебный

```go
func ParseFrame(data []byte) (*Frame, error) {
    // Defensive: минимальный размер фрейма — 2 байта (FIN+opcode, MASK+length)
    if len(data) < 2 {
        return nil, fmt.Errorf("frame too short: %d bytes, minimum 2", len(data))
    }

    opcode := data[0] & 0x0F

    // Defensive: reserved opcodes
    if opcode >= 3 && opcode <= 7 {
        return nil, fmt.Errorf("reserved data opcode: %d", opcode)
    }
    if opcode >= 11 && opcode <= 15 {
        return nil, fmt.Errorf("reserved control opcode: %d", opcode)
    }

    // Defensive: control frames не могут быть фрагментированы
    fin := data[0]&0x80 != 0
    if opcode >= 8 && !fin {
        return nil, fmt.Errorf("fragmented control frame (opcode %d)", opcode)
    }

    // Defensive: control frames max 125 bytes
    payloadLen := int(data[1] & 0x7F)
    if opcode >= 8 && payloadLen > 125 {
        return nil, fmt.Errorf("control frame payload too large: %d > 125", payloadLen)
    }

    // ...
}
```

Каждая проверка — граница доверия. Данные из сети **никогда** не trusted.

---

## Сравнение с Python

| Аспект | Go | Python (pytest) | SQLite (C) |
|--------|-----|-----------------|-----------|
| Фаззинг | `testing.F` встроен | `hypothesis` (property) + AFL | AFL, OSS-Fuzz, dbsqlfuzz, jfuzz |
| Race detection | `go test -race` | Нет | Mutex asserts |
| Coverage | `go test -cover` (statement) | `pytest-cov` (statement+branch) | gcov (branch + MC/DC) |
| Assertions | Build tags + manual | `assert` (всегда вкл.) | `assert()` (ifdef NDEBUG) |
| Mutation testing | `gremlins` | `mutmut`, `cosmic-ray` | Custom script |
| Leak detection | `goleak` / manual | Нет (GC) | Встроенное (malloc wrappers) |
| Property testing | `rapid`, `gopter` | `hypothesis` | Custom |
| Fault injection | Интерфейсы + fake | `unittest.mock.patch` | VFS + malloc wrappers |
| Benchmark | `testing.B` встроен | `pytest-benchmark` | `speedtest1.c` |

### Преимущество Go для этого проекта

Python `hypothesis` — мощный инструмент для property-based testing, но не интегрирован с stdlib. Go `testing.F` — часть языка, crashes автоматически становятся regression-тестами в `testdata/fuzz/`.

Go `-race` — **уникальная суперсила**, которой нет ни у Python, ни у C (SQLite использует ручные mutex asserts). ThreadSanitizer под капотом ловит data races, которые проявляются недетерминистически — именно такие баги убивают concurrent системы.

---

## Теоретические истоки

### Dijkstra: "Testing shows the presence, not the absence of bugs" (1970)

Из "Notes on Structured Programming": тестирование **доказывает наличие** багов, но никогда их отсутствие. Исчерпывающее тестирование всех входов невозможно. Ответ SQLite: максимально приблизиться к исчерпывающему через MC/DC + fuzz + property-based.

### DO-178B/C: MC/DC из авиации

MC/DC (Modified Condition/Decision Coverage) — требование стандарта **DO-178B** (1992, обновлён как DO-178C в 2012) для **Level A** авиационного ПО (отказ = потеря самолёта). SQLite — один из немногих open-source проектов, достигающих 100% MC/DC.

Hayhurst, K. et al. "A Practical Tutorial on Modified Condition/Decision Coverage." NASA/TM-2001-210876, 2001.

### QuickCheck: Property-Based Testing (Claessen & Hughes, 2000)

QuickCheck ввёл идею: вместо конкретных тестов формулируй **свойства** (инварианты), а framework генерирует random inputs и при провале **shrinks** до минимального контрпримера. Прямой предок `rapid` и `testing/quick` в Go.

Claessen, K. & Hughes, J. "QuickCheck: A Lightweight Tool for Random Testing of Haskell Programs." *ICFP*, 2000.

### Mutation Testing (DeMillo, Lipton & Sayward, 1978)

"Hints on Test Data Selection: Help for the Practicing Programmer." *IEEE Computer*, 1978. Идея: если тест не ловит мутацию кода (замена `>` на `>=`), тест недостаточно точен. SQLite проверяет это скриптом на ассемблерном уровне.

### Chaos Engineering (Netflix, 2010)

Chaos Monkey — намеренное убийство production-сервисов для проверки устойчивости. SQLite crash testing — та же философия: **намеренно ломай и проверяй восстановление**. Для WebSocket-прокси: убивай соединения, роняй SQLite, обрывай relay — и проверяй что система корректно обрабатывает каждый отказ.

---

## Что читать дальше

### SQLite Testing
- **SQLite Documentation.** ["How SQLite Is Tested."](https://sqlite.org/testing.html) — первоисточник, must read
- **SQLite Documentation.** ["TH3: Test Harness #3."](https://sqlite.org/th3.html) — проприетарный harness, описание подхода

### Теория тестирования
- **Dijkstra, E.W.** "Notes on Structured Programming." *EWD249*, 1970 — "Testing shows the presence, not the absence of bugs"
- **DeMillo, R., Lipton, R., Sayward, F.** "Hints on Test Data Selection." *IEEE Computer* 11(4), 1978 — основа mutation testing
- **Claessen, K. & Hughes, J.** "QuickCheck: A Lightweight Tool for Random Testing of Haskell Programs." *ICFP*, 2000
- **Hayhurst, K. et al.** "A Practical Tutorial on Modified Condition/Decision Coverage." *NASA/TM-2001-210876*, 2001

### Практическое
- **Go Documentation.** ["Fuzzing"](https://go.dev/doc/security/fuzz/) — официальный гайд
- **`rapid` library.** [pgregory.net/rapid](https://pkg.go.dev/pgregory.net/rapid) — property-based testing для Go
- **`goleak` library.** [go.uber.org/goleak](https://pkg.go.dev/go.uber.org/goleak) — goroutine leak detection
- **Basiri, A. et al.** "Chaos Engineering." *IEEE Software* 33(3), 2016 — формализация подхода Netflix
