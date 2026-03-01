# 01 — Concurrency Patterns: Go vs Python

## Зачем этот документ

Конкурентность — одна из главных причин выбора Go для сетевых сервисов. Этот документ объясняет **как Go планирует горутины**, сравнивает с моделью Python и показывает какие паттерны использовать в проекте WebSocket-прокси.

---

## Часть 1: Модель конкурентности Go — GMP

### Три абстракции

Go runtime использует **M:N scheduling** — множество горутин (M) мультиплексируются на N потоков ОС. Три ключевых компонента:

```
     G (Goroutine)          M (Machine)            P (Processor)
     ┌─────────────┐        ┌─────────────┐        ┌─────────────┐
     │ Стек: 2 KB  │        │ OS-тред     │        │ Лок. очередь│
     │ (растёт до  │        │ (реальный   │        │ до 256 G    │
     │  1 GB)      │        │  pthread)   │        │             │
     │ PC, SP      │        │ Нужен P для │        │ mcache      │
     │ Статус      │        │ Go-кода     │        │ таймеры     │
     └─────────────┘        └─────────────┘        └─────────────┘
```

**G (Goroutine)** — единица работы. Это не OS-тред, а user-space "зелёный поток". Каждая горутина:
- Начинается со стеком в **2 KB** (OS-тред — 8 MB)
- Стек растёт динамически до **1 GB** (копирование + обновление указателей)
- Стоит создания ~**1 μs** (OS-тред — 10-100 μs)
- Переключение контекста ~**100-200 ns** (OS-тред — 1-10 μs)
- Общая стоимость: ~2,500 байт (struct + стек)

**M (Machine)** — OS-тред. Создаётся по требованию. Лимит по умолчанию 10,000 (`debug.SetMaxThreads`). M **обязан** иметь P для выполнения Go-кода.

**P (Processor)** — логический процессор. Количество P = `GOMAXPROCS` (по умолчанию `runtime.NumCPU()`). Каждый P владеет:
- **Локальной очередью** до 256 горутин (lock-free ring buffer)
- Per-P кэшем для аллокаций (`mcache`)
- Таймерами

### Как работает планировщик

Когда M ищет работу, вызывается `findRunnable()`:

```
1. Каждый 61-й тик → проверить ГЛОБАЛЬНУЮ очередь
   (предотвращает голодание)

2. Проверить ЛОКАЛЬНУЮ очередь текущего P

3. Проверить ГЛОБАЛЬНУЮ очередь

4. Опросить сетевой poller (epoll/kqueue)
   на готовые I/O-горутины

5. УКРАСТЬ у другого P:
   → выбрать случайный P
   → забрать ПОЛОВИНУ его очереди

6. Если ничего нет → M засыпает (паркуется)
```

Число **1/61** — тюнинг-эвристика: баланс между честностью глобальной очереди и оверхедом на захват мьютекса.

### Три эры планировщика

| Версия | Модель | Проблема |
|--------|--------|----------|
| Go 1.0-1.1 | Кооперативный | `for {}` блокирует тред навсегда |
| Go 1.2-1.13 | Кооперативный + проверки в прологах функций | Тесный цикл без вызовов функций не прерывается |
| **Go 1.14+** | **Асинхронная вытесняющая** | Решено через SIGURG |

**Go 1.14+ (текущая модель):**
1. `sysmon` (фоновый M без P) обнаруживает горутину, бегущую >**10 ms**
2. Посылает **SIGURG** на OS-тред
3. Обработчик сигнала прерывает горутину в **safe point**
4. Сохраняет регистры, запускает yield
5. Другая горутина получает время на этом P

> SIGURG выбран потому что он определён POSIX как "urgent condition on socket" и почти никогда не используется приложениями.

### Блокирующий syscall vs канал

Это **принципиально** разные ситуации:

**Блокирующий syscall (file I/O):**
```
G блокирует M → P отсоединяется от M → P переходит к другому M
→ если свободного M нет, создаётся новый
→ когда syscall завершается, G ищет свободный P
```

**Блокирующий канал:**
```
G паркуется (state = _Gwaiting) → попадает в wait queue канала
→ M+P СРАЗУ берёт другую G из очереди
→ НЕТ нового треда
→ когда канал разблокирован, G возвращается в run queue
```

**Сетевой I/O (особый случай):**
Go runtime конвертирует сетевые операции в non-blocking и использует **netpoller** (`epoll` на Linux, `kqueue` на macOS). Горутина паркуется, M продолжает работу. Поэтому Go легко держит десятки тысяч одновременных соединений.

### Управление стеком

| Версия | Стратегия | Начальный размер |
|--------|-----------|-----------------|
| Go 1.0-1.2 | Сегментированные стеки | 4-8 KB |
| Go 1.3 | Непрерывные (с копированием) | 8 KB |
| **Go 1.4+** | Непрерывные (с копированием) | **2 KB** |

Рост стека:
1. Компилятор вставляет **stack guard check** в пролог каждой функции
2. Если места мало → `runtime.morestack` → аллокация стека **2x** текущего
3. **Копирование** всего старого стека + обновление указателей
4. При GC: если горутина использует **<1/4** стека → стек **урезается вдвое**

Сегментированные стеки были заменены из-за проблемы **"hot split"** — если вызов функции на границе сегмента повторяется в цикле, каждый раз аллоцируется/освобождается новый сегмент.

---

## Часть 2: Модель конкурентности Python

### GIL — Global Interpreter Lock

GIL — мьютекс в CPython, гарантирующий что **только один тред** исполняет Python-байткод в любой момент времени.

**Зачем существует:**
1. **Reference counting** — `ob_refcnt` на каждом `PyObject`. Без GIL каждый инкремент/декремент требовал бы атомарных операций
2. **C-расширения** — авторы не думают о thread safety, GIL защищает
3. **Производительность single-threaded** — fine-grained locking давал ~2x замедление

> GIL — деталь реализации CPython, а не языка Python. Jython и IronPython не имеют GIL.

### Три модели конкурентности Python

```
┌──────────────────────────────────────────────────────────────┐
│                    Python Concurrency                         │
├──────────────────┬──────────────────┬────────────────────────┤
│   threading      │  multiprocessing │      asyncio           │
│                  │                  │                        │
│ OS-треды         │ OS-процессы      │ Корутины               │
│ GIL сериализует  │ Свой GIL у       │ 1 тред, event loop     │
│ CPU-работу       │ каждого процесса │ кооперативное          │
│                  │                  │ переключение           │
│ Подходит для:    │ Подходит для:    │ Подходит для:          │
│ I/O-bound        │ CPU-bound        │ Высокая конкурентность │
│                  │ (тяжёлый IPC)    │ I/O-bound              │
└──────────────────┴──────────────────┴────────────────────────┘
```

### Как GIL переключает треды

**Python 2:** каждые **100 байткод-инструкций** (непредсказуемо — инструкции имеют разное время).

**Python 3.2+ (новый GIL):** временной интервал **5 ms** (`sys.setswitchinterval`):
1. Тред держит GIL, работает до истечения интервала
2. Runtime ставит `gil_drop_request = 1`
3. Тред видит флаг → освобождает GIL → сигналит `switch_cond`
4. Ждущий тред просыпается и захватывает GIL
5. **Гарантия:** тот же тред не может перехватить GIL обратно (решение проблемы "GIL battle")

### Проблема "function coloring"

В Python есть **два мира** — синхронный и асинхронный:

```python
# Синхронный мир
def fetch_data():
    return requests.get(url)

# Асинхронный мир (другой "цвет")
async def fetch_data():
    return await aiohttp.get(url)
```

Нельзя вызвать `async` функцию из обычной без `asyncio.run()`. Нельзя вызвать блокирующую функцию из `async` без `run_in_executor()`. Два мира **не смешиваются** легко.

**В Go этой проблемы нет:** любая функция запускается как горутина через `go f()`. Нет синтаксического различия между "конкурентной" и "обычной" функцией.

### Free-threaded Python (PEP 703)

| Версия | Статус | Оверхед single-threaded |
|--------|--------|------------------------|
| Python 3.13 (окт 2024) | Экспериментальный | ~40% |
| Python 3.14 (окт 2025) | Официально поддерживается (PEP 779) | ~5-10% |
| Python 3.15+ (2026-27) | GIL управляется runtime-флагом | TBD |

Python 3.14 с `--disable-gil` наконец позволяет тредам работать **параллельно**. Но:
- Не все C-расширения thread-safe
- Экосистема адаптируется постепенно
- Нет M:N scheduling — каждый тред = OS-тред (1:1)
- Нет work stealing

---

## Часть 3: Прямое сравнение

### Стоимость единицы конкурентности

| Единица | Память | Практический максимум |
|---------|--------|----------------------|
| Python OS-тред | ~8 MB (стек) | ~1,000-10,000 |
| Python asyncio корутина | ~2-3 KB | ~100,000+ |
| Python multiprocessing процесс | ~30-50 MB | ~10-100 |
| **Go горутина** | **~2-4 KB** (растёт) | **~1,000,000+** |

### Оверхед операций

| Операция | Время |
|----------|-------|
| Python GIL release/acquire | ~1-5 μs |
| Python OS context switch | ~1-10 μs |
| Python asyncio task switch | ~0.1-1 μs |
| Python multiprocessing IPC (1KB pickle) | ~100 μs |
| **Go goroutine context switch** | **~0.1-0.3 μs** |
| **Go channel send (unbuffered)** | **~0.05-0.1 μs** |

### Ключевые различия

| Аспект | Python (классический) | Python 3.14 free-threaded | Go |
|--------|----------------------|--------------------------|-----|
| Параллелизм тредов | Нет (GIL) | Да | Да |
| Лёгкая конкурентность | asyncio (~2-3 KB) | треды (~8 MB) | горутины (~2-4 KB) |
| M:N scheduling | Нет | Нет (1:1) | **Да (M:N + work stealing)** |
| Вытесняющее планирование | asyncio — нет / треды — да, но GIL | Да (OS-level) | Да (runtime-level, эффективнее) |
| Каналы | Библиотека (`Queue`) | Библиотека (`Queue`) | **Встроены в язык** (`chan`) |
| Function coloring | Да (`async`/`await`) | Нет для тредов | **Нет** |
| Race detector | Нет встроенного | Нет встроенного | **`go test -race`** |

### Как Go решает сколько давать времени горутине

Это ключевой вопрос и главное отличие от Python:

**Python:** OS-шедулер решает когда переключить тред (preemptive для тредов) + GIL добавляет свой 5ms-интервал. Asyncio — полностью кооперативный, корутина бежит пока не встретит `await`.

**Go:** Runtime сам управляет планированием на уровне user-space:
1. **Кооперативные точки** — вызовы функций (stack check в прологе), операции с каналами, `runtime.Gosched()`
2. **Вытесняющее прерывание** — `sysmon` следит, и если горутина бежит >10ms, посылает SIGURG
3. **Work stealing** — если P простаивает, он крадёт половину очереди у другого P
4. **Netpoller** — сетевой I/O не блокирует ни один тред

Go runtime по сути — **маленькая ОС внутри процесса**, которая планирует горутины эффективнее чем ОС планирует треды, потому что:
- Знает семантику Go-кода (каналы, select, GC)
- Переключает контекст в user-space (не нужен syscall)
- Сохраняет только ~15 регистров (а не полное состояние CPU с SSE/AVX)

---

## Часть 4: Паттерны для проекта

### Bidirectional relay (фаза 1)

Классический паттерн для WebSocket-прокси — две горутины + done-канал:

```go
func relay(client, server *websocket.Conn, ctx context.Context) error {
    ctx, cancel := context.WithCancel(ctx)
    defer cancel()

    errc := make(chan error, 2)

    // client → server
    go func() {
        for {
            msgType, msg, err := client.ReadMessage()
            if err != nil {
                errc <- fmt.Errorf("client read: %w", err)
                return
            }
            if err := server.WriteMessage(msgType, msg); err != nil {
                errc <- fmt.Errorf("server write: %w", err)
                return
            }
        }
    }()

    // server → client
    go func() {
        for {
            msgType, msg, err := server.ReadMessage()
            if err != nil {
                errc <- fmt.Errorf("server read: %w", err)
                return
            }
            if err := client.WriteMessage(msgType, msg); err != nil {
                errc <- fmt.Errorf("client write: %w", err)
                return
            }
        }
    }()

    // Ждём первую ошибку (или отмену контекста)
    select {
    case err := <-errc:
        return err
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

### Select + timeout

```go
select {
case msg := <-ch:
    process(msg)
case <-time.After(5 * time.Second):
    return errors.New("timeout waiting for message")
case <-ctx.Done():
    return ctx.Err()
}
```

### Context cancellation

```go
// Родительский контекст
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// Дочерний с таймаутом
childCtx, childCancel := context.WithTimeout(ctx, 30*time.Second)
defer childCancel()

// Когда parent отменяется — все дочерние тоже
```

`context.WithCancel` — когда отменяем вручную (пользователь нажал Ctrl+C).
`context.WithTimeout` — когда есть дедлайн (соединение не отвечает 30 секунд).

---

## Ключевой вывод

В Python конкурентность фрагментирована на три парадигмы (threading, multiprocessing, asyncio), каждая со своими ограничениями и трейдоффами. В Go — единая модель: горутины + каналы + `select`. Runtime берёт на себя всю сложность планирования, а разработчик работает с простыми примитивами.

Для WebSocket-прокси это критично: нужно держать тысячи одновременных соединений с минимальным оверхедом. В Python это потребовало бы asyncio (с проблемой function coloring) или multiprocessing (с тяжёлым IPC). В Go — просто `go relay(client, server, ctx)` на каждое соединение.

---

## Теоретические истоки

### CSP — Communicating Sequential Processes (Hoare, 1978)

Go каналы и горутины — прямая реализация CSP. Hoare формализовал модель где **процессы** общаются через **синхронные каналы**, а `select` — это математическая конструкция недетерминированного выбора.

Линейка Bell Labs → Go:
- **Newsqueak** (Pike, 1988) — первые first-class channels, interpreted
- **Alef** (Winterbottom, Plan 9, 1992) — CSP в compiled C-like языке, но без GC → провал
- **Limbo** (Pike et al., Inferno, 1995) — Alef + GC, прямой предок Go
- **Go** (2007-2009) — кульминация 20 лет экспериментов Pike'а с CSP

### CSP vs Actor Model — почему Go выбрал CSP

| | CSP (Go) | Actor Model (Erlang) |
|---|---|---|
| Identity | Горутины анонимны | Акторы имеют имена/адреса |
| Коммуникация | Через именованные **каналы** | Через сообщения именованным акторам |
| Синхронность | По дефолту синхронный (rendezvous) | Фундаментально асинхронный |
| Для чего | Single-machine concurrency | Distributed systems |

Go выбрал CSP потому что: (1) проектировался для серверного кода на одной машине, (2) синхронные каналы дают более сильные гарантии рассуждений, (3) анонимные горутины + именованные каналы = Unix-like композиция.

### M:N Scheduling и Work Stealing (Blumofe & Leiserson, 1999)

Work stealing формализован в "Scheduling Multithreaded Computations by Work Stealing" (JACM, 1999). Алгоритм: idle processor **крадёт половину** очереди у random busy processor. Это математически оптимально для load balancing. Go scheduler реализует ровно это — каждый P крадёт половину LRQ у случайного другого P.

Тот же принцип в **Cilk** (MIT), **Java ForkJoinPool**, **Tokio** (Rust).

### Guarded Commands (Dijkstra, 1975) → Select

Go `select` восходит к **guarded commands** Дейкстры из "Guarded Commands, Nondeterminacy and Formal Derivation of Programs" (1975). Hoare адаптировал это в CSP, язык **occam** (1983, для транспьютеров) дал практическую реализацию, Newsqueak/Alef/Limbo пронесли через Bell Labs, и Go унаследовал в финальной форме.

### Семафоры, мониторы, каналы — эволюция

| Примитив | Автор, год | Go эквивалент |
|----------|-----------|---------------|
| **Семафор** | Dijkstra, 1965 | `sync.Mutex`, `sync.WaitGroup` |
| **Монитор** | Hoare, 1974 | `sync.Mutex` + `sync.Cond` |
| **Канал (CSP)** | Hoare, 1978 | `chan T`, `select` |

Go предоставляет все три уровня, но идиоматически предпочитает каналы.

---

## Что читать дальше

### Обязательное
- **Hoare, C.A.R.** "Communicating Sequential Processes." *Communications of the ACM* 21(8), 1978. — Оригинальная статья CSP
- **Hoare, C.A.R.** *Communicating Sequential Processes.* Prentice Hall, 1985. — Книга, бесплатно на [usingcsp.com](http://www.usingcsp.com/)
- **Cox, Russ.** ["Bell Labs and CSP Threads"](https://swtch.com/~rsc/thread/) — линейка Newsqueak→Alef→Limbo→Go

### Углублённое
- **Blumofe, R. & Leiserson, C.** "Scheduling Multithreaded Computations by Work Stealing." *JACM* 46(5), 1999
- **Hewitt, C., Bishop, P., Steiger, R.** "A Universal Modular ACTOR Formalism for Artificial Intelligence." *IJCAI*, 1973 — Actor model, альтернатива CSP
- **Dijkstra, E.W.** "Guarded Commands, Nondeterminacy and Formal Derivation of Programs." *CACM* 18(8), 1975

### О Go конкретно
- **Pike, Rob.** ["Concurrency is not Parallelism"](https://go.dev/blog/waza-talk) — ключевой доклад
- **Pike, Rob.** "The Implementation of Newsqueak." *Software: Practice and Experience* 20(7), 1990
- **Go Blog:** ["Go Concurrency Patterns: Pipelines and cancellation"](https://go.dev/blog/pipelines) — Sameer Ajmani, 2014
