# 11 — Анализ ниш: где проект может быть уникальным

## Текущий ландшафт инструментов

### Полная карта конкурентов

| Инструмент | Язык | Лицензия | WS-фокус | Перехват | Модификация | Replay | Recording | Фаззинг | Conformance | CLI | Embeddable |
|-----------|------|---------|----------|----------|-------------|--------|-----------|---------|-------------|-----|------------|
| **Chrome DevTools** | C++ | Proprietary | Вторичный | Read-only | Нет | Нет | Нет¹ | Нет | Нет | Нет | Нет |
| **mitmproxy** | Python | MIT | Вторичный | Да | Да (скрипты) | **Нет²** | Частично | Нет | Нет | Да | Нет |
| **Burp Suite Pro** | Java | Commercial ($449/yr) | Вторичный | Да | Да | Да | Да | Да | Нет | Нет | Нет |
| **OWASP ZAP** | Java | Apache-2.0 | Вторичный | Да | Да (addon) | Да | Да | Да | Нет | Частично | Нет |
| **Wireshark** | C | GPL-2.0 | Вторичный | Passive | Нет | Нет | pcap | Нет | Нет | `tshark` | Нет |
| **Charles Proxy** | Java | Commercial ($50) | Вторичный | Да | Read-only | Нет | Да | Нет | Нет | Нет | Нет |
| **Fiddler** | C# | Freemium | Вторичный | Да | Да | Нет | Да | Нет | Нет | Нет | Нет |
| **Proxyman** | Swift | Freemium | Вторичный | Да | Да | Нет | Да | Нет | Нет | Нет | Нет |
| **websocat** | Rust | MIT | **Первичный** | Нет³ | Нет | Нет | Нет | Нет | Нет | **Да** | Нет |
| **WSSiP** | Node.js | MIT | **Первичный** | Да | Да | Нет | Нет | Нет | Нет | Нет | Нет |
| **ws-replay** | Node.js | MIT | **Первичный** | Нет | Нет | **Да** | **Да** | Нет | Нет | Нет | Нет |
| **Autobahn TS** | Python 2.7 | MIT | **Первичный** | Нет | Нет | Нет | Нет | **Да** | **Да (521)** | Нет | Docker |
| **wsproxy (наш)** | Go | TBD | **Первичный** | Planned | Planned | Planned | Planned | Planned | Planned | **Да** | **Planned** |

¹ Chrome DevTools HAR export содержит WS-фреймы через нестандартное расширение `_webSocketMessages`, но это не recording в полном смысле.
² mitmproxy: "Client or server replay is not possible yet" (официальная документация). Открытый issue #6721: невозможно экспортировать WS-сообщения.
³ websocat — CLI-клиент (like curl), не прокси. Подключается к серверу, не стоит между клиентом и сервером.

### Ключевые наблюдения

**1. Ни один инструмент не является WS-first И полнофункциональным.**

WS-first инструменты (websocat, WSSiP, ws-replay) — узкоспециализированные, покрывающие 1-2 функции. Полнофункциональные (Burp, mitmproxy, ZAP) — HTTP-first с WS-поддержкой как secondary.

**2. mitmproxy — главный open-source конкурент — имеет критичные пробелы в WS.**

| Функция | mitmproxy статус | Источник |
|---------|-----------------|---------|
| WS-перехват | Работает | Документация |
| WS-модификация | Работает (через скрипты) | Документация |
| WS-replay | **Не реализован** | Официальная документация |
| WS-export | **Не реализован** | GitHub issue #6721 (март 2024) |
| Ping/Pong хранение | **Не хранит** | Документация: "will forward but not store" |
| Crash при disconnect | **Баг** | GitHub issue #6324 (авг 2023) |

**3. Autobahn TestSuite — единственный conformance tool — де-факто мёртв.**

| Факт | Деталь |
|------|--------|
| Язык | Python **2.7** (end of life с 2020) |
| Runtime | PyPy 7.3.11 на `pypy:2-7-bullseye` |
| Распространение | Только через Docker-образ |
| Последнее обновление логики | Годы назад |
| Интеграция с Go/Rust | Только через Docker → subprocess |
| Native Go API | Не существует |

**4. Нет embeddable WS-тестовой библиотеки.**

Все инструменты — standalone. Ни один нельзя `go get` и использовать как библиотеку в чужих тестах. Ближайший аналог: `net/http/httptest` для HTTP — но для WebSocket такого нет.

---

## Три возможные ниши

### Ниша A: WebSocket Debugging Proxy (CLI)

**Что это:** `wsproxy` — терминальный инструмент для перехвата, записи, replay, редактирования WS-трафика. "mitmproxy, но WS-first и single binary".

**Целевая аудитория:**
- Backend-разработчики, отлаживающие WS-сервисы
- Frontend-разработчики, работающие с WS-API
- DevOps/SRE, диагностирующие проблемы в production

**Конкурентное преимущество:**

| Свойство | mitmproxy | Burp Suite | **wsproxy** |
|----------|-----------|------------|-------------|
| Установка | `pip install` + зависимости | GUI installer + JRE | **`go install` / single binary** |
| WS-replay | Нет | Да | **Да** |
| WS-export | Нет | Да | **Да (HAR, JSONL)** |
| Frame-level control | Message-level | Message-level | **Frame-level** |
| Scripting | Python addon API | Java/Python | **Go plugin / Lua** |
| Размер | ~50 MB + Python runtime | ~400 MB + JRE | **~10-15 MB binary** |
| CI/CD | Сложно | Невозможно | **`wsproxy record` в pipeline** |
| Ping/Pong | Не хранит | Хранит | **Хранит** |

**Уникальные фичи, которых нет ни у кого:**

1. **Frame-level vs message-level** — мы работаем на уровне фреймов, а не сообщений. Видим фрагментацию, control frames между фрагментами, RSV-биты. Все конкуренты работают на message-level.

2. **SQLite-хранилище** — все фреймы в structured БД. `SELECT * FROM frames WHERE session_id = ? AND direction = 'server_to_client' AND payload LIKE '%error%'`. Ни один конкурент не предлагает SQL-доступ к записанному трафику.

3. **Diff двух сессий** — записать сессию до изменения кода и после, сравнить фрейм по фрейму. Аналог `git diff` для WS-трафика.

**Риски:**
- Маленькая аудитория: большинство WS-отладки делается через Chrome DevTools
- mitmproxy может закрыть пробелы в WS-поддержке
- Долгий путь до feature parity с Burp

**Оценка сложности:** Высокая. Полноценный прокси с TUI — месяцы работы.

---

### Ниша B: WebSocket Conformance & Testing Library

**Что это:** `wstest` — Go-библиотека для тестирования WebSocket-реализаций. Замена Autobahn TestSuite. `go get` → использовать в своих тестах.

**Целевая аудитория:**
- Авторы WS-библиотек (gorilla/websocket, nhooyr/websocket, gobwas/ws, и аналоги на Rust/C/C++)
- Разработчики WS-серверов, желающие проверить conformance
- CI/CD pipelines

**Текущее состояние рынка:**

| Инструмент | Язык | API | Кейсов | Интеграция с Go | Поддерживается |
|-----------|------|-----|--------|----------------|---------------|
| Autobahn TS | Python 2.7 | Docker CLI | ~521 | Docker subprocess | Заморожен |
| **wstest (наш)** | **Go** | **`go test` native** | **Planned: 521+** | **Native** | **Активно** |

**Конкретная архитектура:**

```go
// Использование в чужом проекте:
import "github.com/user/wstest/conformance"

func TestMyWSServer(t *testing.T) {
    srv := startMyServer()
    defer srv.Close()

    // Все 521+ тестов RFC 6455
    conformance.RunAll(t, srv.URL)
}

func TestMyWSServerSpecific(t *testing.T) {
    srv := startMyServer()
    defer srv.Close()

    // Только конкретные категории
    conformance.Run(t, srv.URL,
        conformance.Framing,
        conformance.UTF8Handling,
        conformance.CloseHandshake,
    )
}
```

**Категории тестов Autobahn, которые нужно покрыть:**

| # | Категория | Что тестируется | Кол-во кейсов (Autobahn) |
|---|----------|----------------|--------------------------|
| 1 | Framing | Заголовки фреймов, opcodes, payload length, masking | ~30 |
| 2 | Ping/Pong | Ping с payload, pong-ответы, unsolicited pong, ping между фрагментами | ~10 |
| 3 | Reserved Bits | RSV1-3 = 0 без расширений, невалидные комбинации | ~10 |
| 4 | Opcodes | Валидные/невалидные opcodes, continuation frames | ~10 |
| 5 | Fragmentation | Фрагментация text/binary, interleaved control frames, невалидные последовательности | ~20 |
| 6 | UTF-8 Handling | Валидный/невалидный UTF-8, partial sequences, overlong encodings | ~150+ |
| 7 | Close Handling | Close codes, close payloads, close handshake, abnormal closures | ~30 |
| 8 | Limits/Performance | Макс. frame size, макс. message size, производительность | ~20 |
| 9 | Compression | permessage-deflate, параметры, context takeover (RFC 7692) | ~200+ |

**Что мы можем добавить поверх Autobahn:**

| Категория | Описание | Почему Autobahn не покрывает |
|-----------|----------|------------------------------|
| **Fuzz** | Случайные мутации фреймов, truncated frames, garbage bytes | Autobahn = deterministic, наш = fuzz |
| **Fault injection** | Обрыв на N-м фрейме, slow writes, partial frames | Autobahn не тестирует robustness к сбоям |
| **Timing** | Медленные/быстрые потоки, burst, timeout handling | Autobahn не тестирует temporal behaviour |
| **Concurrent** | Множество одновременных соединений, race conditions | Autobahn = один клиент за раз |
| **Security** | Oversized frames (DoS), CSWSH, origin validation | Autobahn = conformance, не security |

**Риски:**
- Написать 521+ качественных тестов — огромная работа
- Нужно проверять корректность тестов на нескольких WS-библиотеках
- Маленькая аудитория (авторы WS-библиотек — десятки людей)

**Оценка сложности:** Средняя-высокая. Тесты можно добавлять инкрементально, начиная с самых важных категорий.

---

### Ниша C: WebSocket Fault Injection Library

**Что это:** `wsfault` — embeddable Go-библиотека для chaos testing WS-приложений. "SQLite-style anomaly testing для WebSocket."

**Целевая аудитория:**
- Go-разработчики, пишущие WS-серверы и клиенты
- Команды, внедряющие chaos engineering
- CI/CD pipelines для WebSocket-сервисов

**Текущее состояние: пусто.**

Нет ни одного инструмента, который предлагает программируемый fault injection для WebSocket. Для HTTP есть `toxiproxy` (Shopify), `chaos-monkey`, `litmus`. Для WebSocket — **ничего**.

**Аналогия с SQLite testing:**

| SQLite механизм | WS-аналог | Реализация |
|----------------|-----------|-----------|
| `SQLITE_CONFIG_MALLOC` (подмена malloc) | `wsfault.Conn` (обёртка над WS conn) | Интерфейс `websocket.Conn` → faulty wrapper |
| Fail на N-й аллокации | Fail на N-м фрейме | `wsfault.FailAfterFrames(n)` |
| I/O Error VFS | Network error injection | `wsfault.InjectReadError(err)` |
| Crash testing (kill process) | Connection abort | `wsfault.AbruptClose()` (без close handshake) |
| Compound failures | Cascading faults | `wsfault.Chain(SlowWrite(100ms), FailAfter(10))` |

**Конкретная архитектура:**

```go
import "github.com/user/wstest/fault"

func TestMyAppHandlesSlowServer(t *testing.T) {
    // Обернуть реальное соединение в faulty-wrapper
    realConn, _, _ := websocket.DefaultDialer.Dial(serverURL, nil)

    faultyConn := fault.Wrap(realConn,
        fault.SlowRead(200 * time.Millisecond),  // каждое чтение +200ms
    )

    err := myApp.HandleConnection(faultyConn)
    // Проверить: приложение корректно обработало slow connection
}

func TestMyAppHandlesDisconnectLoop(t *testing.T) {
    // SQLite-style: тест на каждой возможной точке отказа
    for n := 1; n <= 50; n++ {
        t.Run(fmt.Sprintf("disconnect_at_%d", n), func(t *testing.T) {
            conn := fault.Wrap(realConn,
                fault.DisconnectAfterFrames(n),
            )
            err := myApp.HandleConnection(conn)
            assertGracefulDegradation(t, err)
        })
    }
}

func TestMyAppHandlesMalformedFrames(t *testing.T) {
    conn := fault.Wrap(realConn,
        fault.CorruptEveryNthFrame(5, fault.CorruptPayloadLength),
    )
    err := myApp.HandleConnection(conn)
    // Приложение должно закрыть соединение с протокольной ошибкой
}
```

**Каталог fault-ов:**

| Категория | Fault | Параметры | Что проверяет |
|-----------|-------|-----------|--------------|
| **Timing** | `SlowRead(d)` | Задержка на каждом Read | Timeout handling |
| | `SlowWrite(d)` | Задержка на каждом Write | Write timeout |
| | `JitterRead(min, max)` | Случайная задержка | Стабильность под нагрузкой |
| | `PauseAfterFrames(n, d)` | Пауза после N фреймов на D | Keep-alive / ping обнаружение |
| **Disconnect** | `DisconnectAfterFrames(n)` | TCP close после N фреймов | Graceful degradation |
| | `AbruptClose()` | Close без close handshake | Обработка code 1006 |
| | `ResetConnection()` | TCP RST | Жёсткий обрыв |
| **Corruption** | `CorruptEveryNthFrame(n, how)` | Мутация N-го фрейма | Валидация входных данных |
| | `TruncateFrame(maxBytes)` | Обрезать payload | Обработка partial frames |
| | `InvalidUTF8InTextFrame()` | Non-UTF-8 в opcode 1 | RFC 6455 §5.6 compliance |
| | `InvalidCloseCode(code)` | Close с невалидным кодом | Close handling |
| | `OversizedControlFrame(size)` | Control frame > 125 bytes | Frame validation |
| **Backpressure** | `NeverRead()` | Не читать из conn | Write buffer overflow |
| | `ReadEveryNth(n)` | Читать только каждый N-й | Consumer lag |
| **Protocol** | `SendUnmaskedFromClient()` | Client frame без маски | MASK bit validation |
| | `SendMaskedFromServer()` | Server frame с маской | Unexpected masking |
| | `FragmentControlFrame()` | FIN=0 на control frame | Fragmentation rules |

**Риски:**
- Нужно выбрать правильный уровень абстракции (frame vs message vs conn)
- WebSocket conn в Go (`gorilla/websocket.Conn`) — concrete struct, не интерфейс. Нужен adapter
- Маленькая, хоть и благодарная аудитория

**Оценка сложности:** Средняя. Каждый fault — относительно простая обёртка. Ценность в каталоге и удобстве API.

---

## Сравнительный анализ ниш

### Матрица оценки

| Критерий | Вес | A: Debug Proxy | B: Conformance Suite | C: Fault Injection |
|----------|-----|---------------|---------------------|-------------------|
| **Уникальность** | 25% | Средняя — mitmproxy+Burp существуют | Высокая — Autobahn мёртв | **Очень высокая — ничего нет** |
| **Размер аудитории** | 20% | **Большой** — все WS-разработчики | Маленький — авторы WS-библиотек | Средний — Go WS-разработчики |
| **Встраиваемость** | 15% | Низкая — standalone CLI | **Высокая** — `go test` native | **Высокая** — `go test` native |
| **Первый полезный результат** | 15% | Долго — нужен рабочий прокси | Средне — 50 тестов уже полезны | **Быстро — 5 faults уже полезны** |
| **Сложность реализации** | 15% | Высокая | Средняя-высокая | **Средняя** |
| **Учебная ценность** | 10% | **Очень высокая** — net, concurrency, TUI | Высокая — RFC, протоколы | Средняя — интерфейсы, обёртки |

### Scoring (1-5)

| Критерий | A: Debug Proxy | B: Conformance Suite | C: Fault Injection |
|----------|---------------|---------------------|-------------------|
| Уникальность (×25%) | 3 = 0.75 | 4 = 1.00 | **5 = 1.25** |
| Аудитория (×20%) | **5 = 1.00** | 2 = 0.40 | 3 = 0.60 |
| Встраиваемость (×15%) | 1 = 0.15 | **5 = 0.75** | **5 = 0.75** |
| Time to value (×15%) | 2 = 0.30 | 3 = 0.45 | **5 = 0.75** |
| Сложность¹ (×15%) | 2 = 0.30 | 3 = 0.45 | **4 = 0.60** |
| Учебная ценность (×10%) | **5 = 0.50** | 4 = 0.40 | 3 = 0.30 |
| **Итого** | **3.00** | **3.45** | **4.25** |

¹ Инвертировано: 5 = легко реализовать, 1 = очень сложно.

### Визуализация

```
                    Уникальность
                         5
                         │
                    4    │
                   ╱·····•····╲  C: Fault Injection
                  ╱ ·   │   · ╲
Учебная     3────╱──•···│···•──╲────3 Аудитория
ценность        ╲·  ·   │  ·  ╱
                 ╲ · ·   │· · ╱
                  ╲··•···•··╱
                    2    │ 2
                         │
                    Time to value

           A = Debug Proxy  ─── учебная ценность max, но долго
           B = Conformance  ─── уникально, но узкая аудитория
           C = Fault Inject ─── быстро, уникально, встраиваемо
```

---

## Стратегия: всё три, но в правильном порядке

Ниши **не взаимоисключающие**. Они выстраиваются в цепочку, где каждая следующая использует предыдущую:

```
Фаза 1                  Фаза 2                    Фаза 3
┌───────────────┐       ┌───────────────────┐      ┌────────────────┐
│ C: wsfault    │──────▶│ B: wsconformance  │─────▶│ A: wsproxy CLI │
│               │       │                   │      │                │
│ Fault inject  │       │ 521+ RFC 6455     │      │ Full debugging │
│ library       │       │ test cases        │      │ proxy          │
│               │       │ (uses wsfault     │      │ (uses both     │
│ 5-10 fault    │       │  internally)      │      │  internally)   │
│ types         │       │                   │      │                │
│               │       │ + fuzz category   │      │ TUI, replay,   │
│ Time: 2-4 нед │       │ + timing category │      │ edit, HAR      │
│               │       │                   │      │                │
│ Полезен сразу │       │ Time: 2-3 мес     │      │ Time: 4-6 мес  │
└───────────────┘       └───────────────────┘      └────────────────┘
     ▲                         ▲                         ▲
     │                         │                         │
 go get wstest/fault      go get wstest/conformance   go install wsproxy
 Другие проекты           WS-библиотеки              Разработчики
 используют               тестируются                 отлаживают
```

### Почему этот порядок

**1. Fault injection первой** потому что:
- Минимальный scope: 10-15 типов fault-ов = полезная библиотека
- Нужен для conformance suite (фаза 2 использует fault injection в категориях Fuzz, Timing, Concurrent)
- Нужен для прокси (фаза 3 использует для собственных тестов)
- Быстро приносит пользу другим проектам
- Самая высокая уникальность — нулевая конкуренция

**2. Conformance suite второй** потому что:
- Строится поверх fault injection
- Требует глубокого понимания RFC 6455 (которое нужно и для прокси)
- Каждая категория — инкрементальный прогресс (начать с Framing, потом Close, потом UTF-8...)
- Заполняет реальную дыру: Autobahn мёртв

**3. Proxy CLI последним** потому что:
- Использует обе библиотеки для собственного тестирования
- Самый сложный компонент (TUI, storage, relay, CLI, pub/sub)
- Имеет наибольшую учебную ценность (цель проекта — изучение Go)
- К моменту начала прокси, библиотеки уже протестированы и стабильны

---

## Структура репозитория

```
net_proxy_tools/
│
├── wstest/                          ← PUBLIC LIBRARY (go get)
│   │
│   ├── fault/                       ← Фаза 1: Fault Injection
│   │   ├── fault.go                 # Conn wrapper, основные типы
│   │   ├── timing.go                # SlowRead, SlowWrite, Jitter, Pause
│   │   ├── disconnect.go            # AbruptClose, Reset, DisconnectAfter
│   │   ├── corrupt.go               # CorruptFrame, TruncateFrame, InvalidUTF8
│   │   ├── backpressure.go          # NeverRead, ReadEveryNth
│   │   ├── protocol.go              # UnmaskedClient, MaskedServer, FragCtrl
│   │   ├── chain.go                 # Chain(fault1, fault2, ...) композиция
│   │   └── fault_test.go
│   │
│   ├── conformance/                 ← Фаза 2: Conformance Suite
│   │   ├── suite.go                 # RunAll(), Run(categories...)
│   │   ├── category.go              # Framing, PingPong, UTF8, Close, ...
│   │   ├── 01_framing_test.go
│   │   ├── 02_pingpong_test.go
│   │   ├── 03_reserved_bits_test.go
│   │   ├── 04_opcodes_test.go
│   │   ├── 05_fragmentation_test.go
│   │   ├── 06_utf8_test.go
│   │   ├── 07_close_test.go
│   │   ├── 08_limits_test.go
│   │   ├── 09_compression_test.go
│   │   ├── 10_fuzz_test.go          # Наша категория, не из Autobahn
│   │   ├── 11_timing_test.go        # Наша категория
│   │   ├── 12_concurrent_test.go    # Наша категория
│   │   └── 13_security_test.go      # Наша категория
│   │
│   └── record/                      ← Запись/воспроизведение сессий
│       ├── recorder.go              # Перехват фреймов
│       ├── player.go                # Replay с таймингами
│       └── har.go                   # HAR export/import
│
├── internal/                        ← PRIVATE (proxy internals)
│   ├── core/
│   │   └── event.go                 # Frame, Session, Direction, State
│   ├── storage/
│   │   └── store.go                 # SQLite persistence
│   ├── proxy/
│   │   ├── relay.go                 # Bidirectional frame relay
│   │   └── interceptor.go           # Frame inspection/modification
│   └── pubsub/
│       └── bus.go                   # Fan-out event distribution
│
├── cmd/wsproxy/                     ← CLI TOOL (go install)
│   └── main.go
│
└── docs/
```

### Ключевой принцип: `wstest/` — public API, `internal/` — private

`wstest/` — самостоятельная библиотека без зависимости от `internal/`. Другие проекты могут `go get` только `wstest/fault` без затягивания SQLite, Cobra, и всего прокси.

---

## Потенциальные пользователи по нишам

### Ниша C (Fault Injection) — кто будет использовать

| Проект | Язык | GitHub Stars | Зачем им wsfault |
|--------|------|-------------|-----------------|
| gorilla/websocket | Go | ~22K | Тестирование robustness |
| nhooyr/websocket (coder/websocket) | Go | ~3.5K | Тестирование graceful degradation |
| gobwas/ws | Go | ~6K | Low-level, нуждается в fault testing |
| centrifugal/centrifuge | Go | ~4K | Real-time messaging, chaos testing |
| olahol/melody | Go | ~3K | WS framework, нет fault testing |
| Любой Go WS-сервер | Go | — | CI/CD integration |

### Ниша B (Conformance) — кто будет использовать

| Проект | Текущий conformance testing | Наше преимущество |
|--------|----------------------------|-------------------|
| gorilla/websocket | Autobahn через Docker | Native `go test` |
| nhooyr/websocket | Autobahn через Docker | Native `go test`, нет Python 2.7 |
| tokio-tungstenite (Rust) | Autobahn через Docker | Не прямое (но Docker-alternative) |
| uWebSockets (C++) | Autobahn через Docker | Docker-alternative |
| ws (Node.js) | Autobahn через Docker | Docker-alternative |

**Ключевой инсайт:** каждая серьёзная WS-библиотека использует Autobahn, и каждая запускает его через Docker. Go-native альтернатива = прямая ценность для Go-экосистемы, косвенная — для всех остальных (через Docker-формат).

### Ниша A (Proxy) — кто будет использовать

Широкая аудитория: любой разработчик, работающий с WebSocket. Но конкурирует с mitmproxy и Burp — дифференциация через WS-first фокус и single binary.

---

## HAR и формат записи

### Проблема HAR для WebSocket

HAR (HTTP Archive) — стандарт для HTTP-трафика. WebSocket-поддержка **нестандартна**:

| Аспект | Статус |
|--------|--------|
| Спецификация HAR 1.2 | WebSocket не упоминается |
| Chrome DevTools | Нестандартное расширение `_webSocketMessages` (underscore = private) |
| Стандартизация WS в HAR | Предложение от 2015, **не принято** |
| Формат `_webSocketMessages` | Массив {type, time, opcode, data} внутри entries |

**Возможность:** определить и задокументировать **открытый формат** для записи WS-сессий. Не пытаться расширить HAR (политически сложно), а создать свой `.wsar` (WebSocket Archive) — JSON-формат, вдохновлённый HAR, но WS-native:

```json
{
  "version": "1.0",
  "sessions": [{
    "id": "550e8400-...",
    "target": "ws://example.com/chat",
    "startedAt": "2025-03-01T12:00:00Z",
    "closedAt": "2025-03-01T12:05:00Z",
    "closeCode": 1000,
    "frames": [
      {
        "timestamp": "2025-03-01T12:00:00.123Z",
        "direction": "client_to_server",
        "fin": true,
        "opcode": 1,
        "masked": true,
        "payloadLength": 13,
        "payload": "Hello, World!"
      }
    ]
  }]
}
```

Это третья потенциальная "фундаментальная" вещь: **формат**, который другие инструменты могут принять. Форматы живут дольше инструментов.

---

## Реалистичная оценка impact

### Чем проект НЕ станет

- Не станет "следующим SQLite" — аудитория на порядки меньше
- Не заменит mitmproxy или Burp Suite — у них 10+ лет развития и тысячи контрибьюторов
- Не станет стандартом отрасли за 1 год

### Чем проект МОЖЕТ стать

| Цель | Реалистичность | Горизонт |
|------|---------------|---------|
| Лучшая Go-библиотека для WS fault injection | **Высокая** — конкуренции нет | 1-3 мес |
| Замена Autobahn TestSuite для Go | **Средне-высокая** — Autobahn мёртв | 3-6 мес |
| Стандартный инструмент для WS-тестирования в Go-экосистеме | **Средняя** — нужно community adoption | 6-12 мес |
| Полноценный WS debugging proxy | **Средняя** — конкуренция с mitmproxy | 12+ мес |
| Формат `.wsar` принят другими инструментами | **Низкая** — стандарты тяжело продвигать | 1-2 года |

---

## Что читать дальше

### Конкуренты и рынок
- **mitmproxy WS documentation.** [Protocols](https://docs.mitmproxy.org/stable/concepts/protocols/) — ограничения WS-поддержки
- **mitmproxy GitHub issue #6721.** [Export WebSocket messages](https://github.com/mitmproxy/mitmproxy/issues/6721) — нереализованный экспорт
- **Autobahn TestSuite.** [GitHub](https://github.com/crossbario/autobahn-testsuite) — текущее состояние
- **websocat.** [GitHub](https://github.com/vi/websocat) — CLI WS-клиент

### Формат и стандарты
- **W3C HAR spec.** [HTTP Archive format](https://w3c.github.io/web-performance/specs/HAR/Overview.html)
- **Keysight.** ["Looking into WebSocket Traffic in HAR Capture"](https://www.keysight.com/blogs/en/tech/nwvs/2022/07/23/looking-into-websocket-traffic-in-har-capture) — проблемы WS в HAR
- **Google Groups.** [Web Sockets in HAR file: proposal](https://groups.google.com/g/http-archive-specification/c/_DBaSKch_-s) — непринятое предложение

### Chaos Engineering
- **Shopify/toxiproxy.** [GitHub](https://github.com/Shopify/toxiproxy) — TCP fault injection proxy (HTTP, не WS)
- **Netflix.** "Chaos Engineering." *O'Reilly*, 2020
