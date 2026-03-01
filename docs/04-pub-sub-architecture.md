# 04 — Pub/Sub и Event Architecture

## Контекст проекта

WebSocket-прокси записывает фреймы и раздаёт их потребителям: TUI, file writer, GUI. Центральный вопрос — как распределять события от ядра ко множеству подписчиков.

---

## Fan-Out через каналы

Fan-out = **одно сообщение → все подписчики**. Каждый подписчик получает свой канал.

```go
type Bus struct {
    mu   sync.RWMutex
    subs []chan Event
}

func (b *Bus) Subscribe() <-chan Event {
    b.mu.Lock()
    defer b.mu.Unlock()
    ch := make(chan Event, 64)
    b.subs = append(b.subs, ch)
    return ch
}

func (b *Bus) Publish(e Event) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    for _, ch := range b.subs {
        select {
        case ch <- e:
        default:
            // подписчик не успевает — drop
        }
    }
}
```

**Ключевые решения:**
- `<-chan Event` (receive-only) в return — подписчик не может случайно опубликовать в канал
- `RWMutex` — `Publish` берёт read lock, `Subscribe` берёт write lock. Множество Publish могут работать параллельно
- Buffer 64 — поглощает кратковременные всплески

### Buffered vs Unbuffered

| Свойство | Unbuffered | Buffered (N) |
|----------|------------|-------------|
| Send блокирует когда | Нет receiver | Буфер полон |
| Pub/sub риск | Один медленный subscriber блокирует ВСЕ publish | Терпит всплески; блокирует когда полон |

Для pub/sub unbuffered почти всегда неправильный выбор — один медленный подписчик убивает систему.

---

## Backpressure: четыре стратегии

### 1. Block (дефолт каналов)

```go
ch <- msg // блокирует пока подписчик не прочитает
```

Backpressure распространяется вверх — publisher замедляется до скорости самого медленного consumer. **Неприемлемо для fan-out.**

### 2. Drop (non-blocking send)

```go
select {
case ch <- msg:
default:
    log.Println("dropping message for slow subscriber")
}
```

Publisher никогда не блокируется. Потеря сообщений допустима для: метрик, live-дашбордов, TUI. **Наш выбор для TUI/stdout.**

### 3. Buffer (большой буферизованный канал)

```go
ch := make(chan Event, 1000)
```

Гибрид: поглощает временные пики, но в конце концов блокирует или нужно комбинировать с drop.

### 4. Ring Buffer (перезапись старого)

```go
select {
case out <- msg:
default:
    <-out      // удалить старейшее
    out <- msg // вставить новейшее
}
```

Publisher никогда не блокируется, consumer всегда видит самые свежие данные. Теряется старое, а не новое.

---

## Ring Buffer подробнее

### Когда ring buffer лучше каналов

- Нужны **последние N** сообщений (подключение к уже идущей сессии)
- Медленный consumer **не должен** влиять на publisher
- Важнее актуальность, чем полнота

### Channel-based ring buffer

Из CloudFoundry Loggregator:

```go
type RingBuffer struct {
    input  <-chan int
    output chan int
}

func (rb *RingBuffer) Run() {
    for msg := range rb.input {
        select {
        case rb.output <- msg:
        default:
            <-rb.output     // drop oldest
            rb.output <- msg // insert newest
        }
    }
    close(rb.output)
}
```

### Когда оставаться на каналах

- Потеря сообщений неприемлема
- Простая work queue (каждый элемент обрабатывается один раз)
- Нет требования "увидеть последние N фреймов"

---

## Channel Directions для type safety

```go
func publisher(out chan<- Event) {  // может только отправлять
    out <- Event{...}
    // <-out  // ошибка компиляции
}

func subscriber(in <-chan Event) {  // может только получать
    for e := range in {
        process(e)
    }
    // in <- Event{}  // ошибка компиляции
}
```

Bidirectional `chan Event` автоматически конвертируется в нужном направлении при передаче в функцию.

---

## Select паттерны

### Non-blocking send

```go
select {
case ch <- msg:
default:
    // канал полон — пропускаем
}
```

### Timeout

```go
select {
case result := <-ch:
    process(result)
case <-time.After(3 * time.Second):
    return errors.New("timeout")
}
```

### Context-based cancellation

```go
select {
case msg := <-messages:
    process(msg)
case <-ctx.Done():
    return ctx.Err()
}
```

### Priority select

Go `select` выбирает случайно среди готовых cases. Для приоритета — вложенные select:

```go
select {
case msg := <-highPriority:
    handle(msg)
default:
    select {
    case msg := <-highPriority:
        handle(msg)
    case msg := <-lowPriority:
        handle(msg)
    }
}
```

---

## Context-based отписка

Подписчики должны быть отменяемы — WebSocket отключился, сервер выключается:

```go
func (b *Bus) Subscribe(ctx context.Context) <-chan Event {
    b.mu.Lock()
    ch := make(chan Event, 64)
    b.subs = append(b.subs, ch)
    b.mu.Unlock()

    go func() {
        <-ctx.Done()
        b.mu.Lock()
        for i, sub := range b.subs {
            if sub == ch {
                b.subs = append(b.subs[:i], b.subs[i+1:]...)
                break
            }
        }
        b.mu.Unlock()
        close(ch)
    }()

    return ch
}
```

`close(ch)` завершает `range` цикл подписчика. Горутина мониторинга завершается когда контекст отменяется — нет утечек.

---

## Гарантии доставки

| Гарантия | Реализация | Потери | Дубликаты |
|----------|-----------|--------|-----------|
| **At-most-once** | `select/default` drop | Да | Нет |
| **At-least-once** | Ack mechanism + retry | Нет | Да |
| **Exactly-once** | Теоретически невозможно в distributed; at-least-once + идемпотентность | Нет | Нет |

### Наш выбор для проекта

- **TUI/stdout** — at-most-once (drop). Потеря кадра некритична для отображения
- **File writer** — часть ядра, а не подписчик. Записывает напрямую, гарантируя персистентность
- **SQLite store** — транзакционная запись, гарантированная доставка

Это ключевое архитектурное решение из `Intro.md`: file writer **не подписчик**, а часть ядра.

---

## Сравнение с другими языками

| Фича | Go channels | Python asyncio.Queue | Node EventEmitter | Rust mpsc |
|------|-------------|---------------------|-------------------|-----------|
| Модель | CSP, M:N | Кооперативные корутины | Single-thread event loop | OS threads / async runtime |
| Буфер | Явный при создании | Явный (0 = безлимит) | Нет (синхронный) | Bounded / unbounded |
| Backpressure | Нативный (блокирующий send) | `await put()` блокирует корутину | Нет | Bounded channel блокирует |
| MPMC | Да (встроен) | N/A (single-threaded) | N/A (observer pattern) | MPSC only (std); MPMC через crossbeam |
| Select/multiplex | Встроенный `select` | `asyncio.wait` | N/A | `tokio::select!` макрос |
| Оверхед на "задачу" | ~2 KB горутина | ~1 KB корутина | Callback (минимум) | OS thread ~8 MB / async task ~few KB |

**Node EventEmitter** — принципиально другой: нет буфера, синхронная доставка callback'ов, нет backpressure. Медленный listener блокирует весь event loop.

**Python asyncio.Queue** — ближе к Go channels, но single-threaded. `maxsize=0` значит безлимитный буфер (нет backpressure по дефолту!).

**Rust mpsc** — ownership-based. Отправленное значение перемещается (move semantics), sender не может его использовать после отправки. Сильнее Go по типобезопасности, но нет встроенного runtime scheduler.

---

## Теоретические истоки

### Kahn Process Networks (Gilles Kahn, 1974)

Fan-out через каналы формально описывается **Kahn Process Networks** — "The Semantics of a Simple Language for Parallel Programming" (*IFIP Congress*, 1974). KPN: детерминированные процессы + FIFO-каналы + чтение блокирует. Go горутины + каналы — почти точная реализация KPN, с отличием: Go каналы bounded (KPN предполагает unbounded).

### Dataflow Programming (Jack Dennis, MIT, 1974)

Fan-out/fan-in — паттерн из **dataflow computation**: инструкции исполняются когда их данные готовы (data-driven), а не по program counter (control-driven). Dennis & Misunas, "A Preliminary Architecture for a Basic Data-Flow Processor", *ISCA*, 1975.

Go каналы реализуют dataflow: горутина блокируется пока вход не готов, затем исполняется и производит выход.

### Linear Types и "Share by Communicating"

Go прoverб "share memory by communicating" связан с **linear types** (Girard, 1987) и **uniqueness types** (Clean language). Когда значение отправляется в канал, sender conceptually передаёт ownership. Go не enforce это типами (в отличие от Rust), но runtime копирует значение из стека sender в стек receiver.

### io.Reader/Writer как Unix Pipes (McIlroy, 1964)

`io.Reader`/`io.Writer` — in-process аналог Unix pipes. Doug McIlroy предложил pipes в 1964, Ken Thompson реализовал `pipe()` в Unix V3 (1973). `io.TeeReader`, `io.MultiWriter`, `io.Pipe` — это **function composition** из ФП, выраженная через streaming interfaces.

---

## Что читать дальше

- **Kahn, G.** "The Semantics of a Simple Language for Parallel Programming." *IFIP Congress*, 1974
- **Dennis, J.B. & Misunas, D.P.** "A Preliminary Architecture for a Basic Data-Flow Processor." *ISCA*, 1975
- **Go Blog.** ["Pipelines and cancellation."](https://go.dev/blog/pipelines) Sameer Ajmani, 2014
- **Eli Bendersky.** ["PubSub using channels in Go."](https://eli.thegreenplace.net/2020/pubsub-using-channels-in-go/) 2020
- **VMware Tanzu Blog.** ["A channel-based ring buffer in Go."](https://blogs.vmware.com/tanzu/a-channel-based-ring-buffer-in-go/) (CloudFoundry Loggregator)
