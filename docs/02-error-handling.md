# 02 — Error Handling в Go

## Философия: ошибки как значения

Go принципиально отказался от exceptions. Ошибки — обычные возвращаемые значения, с которыми можно работать как с любыми данными.

**Rob Pike (2015):** *"Errors are values. Values can be programmed, and since errors are values, errors can be programmed."*

Три аргумента за такой подход:
1. **Видимость** — ошибки в сигнатуре функции. Не надо читать исходники чтобы узнать что может упасть
2. **Нет скрытого control flow** — видно каждую точку где выполнение может разойтись
3. **Нет stack unwinding** — возврат значения дешевле чем throw/catch

---

## Интерфейс `error`

```go
type error interface {
    Error() string
}
```

Минимален намеренно. Любой тип с методом `Error() string` является ошибкой. Не нужен импорт — `error` предопределён в языке.

---

## Sentinel Errors

Package-level переменные для конкретных, хорошо известных ситуаций:

```go
var (
    ErrNotFound     = errors.New("not found")
    ErrUnauthorized = errors.New("unauthorized")
)
```

**Конвенции:**
- Переменные начинаются с `Err`: `ErrNotFound`, `ErrConflict`
- Текст ошибки **НЕ** содержит слово "error" (это уже ясно из типа)
- Создаются через `errors.New`

**Примеры из стандартной библиотеки:**
- `io.EOF` — конец входных данных
- `sql.ErrNoRows` — запрос не вернул строк
- `os.ErrNotExist` — файл не существует
- `context.Canceled` — контекст отменён

**Когда использовать:** ошибка представляет условие, которое вызывающий код регулярно проверяет, и не несёт дополнительного контекста.

---

## Custom Error Types

Когда ошибке нужны структурированные данные:

```go
type NotFoundError struct {
    Resource string
    ID       int64
}

func (e *NotFoundError) Error() string {
    return fmt.Sprintf("%s with id %d not found", e.Resource, e.ID)
}
```

**Конвенция:** имена типов заканчиваются на `Error` (`PathError`, `ValidationError`). Sentinel-переменные начинаются с `Err`. Не путать.

**Wrappable custom errors** (реализуют `Unwrap`):

```go
type QueryError struct {
    Query string
    Err   error
}

func (e *QueryError) Error() string {
    return fmt.Sprintf("query %q failed: %v", e.Query, e.Err)
}

func (e *QueryError) Unwrap() error {
    return e.Err
}
```

### Когда что выбирать

| Custom Error Type | Sentinel Error |
|-------------------|---------------|
| Нужны данные (коды, поля, ID) | Достаточно простой проверки |
| Нужны доп. методы (`Timeout()`) | Нет поведения кроме идентичности |
| Оборачивает причину | Самостоятельное условие |

---

## Wrapping: `fmt.Errorf` с `%w`

Go 1.13 ввёл wrapping — добавление контекста с сохранением оригинальной ошибки:

```go
originalErr := errors.New("connection refused")
wrappedErr := fmt.Errorf("failed to fetch user profile: %w", originalErr)
```

`%w` делает оригинал доступным через `errors.Unwrap`, создаёт **error chain**.

**`%w` vs `%v`:**
- `%w` — оборачивает (сохраняет для `errors.Is/As`). Оригинал становится частью API-контракта
- `%v` — форматирует как строку (оригинал теряется). Скрывает реализацию

**Go 1.20+ — множественное оборачивание:**

```go
err := fmt.Errorf("operation failed: %w, %w", err1, err2)
// Или:
err := errors.Join(err1, err2, err3)
```

---

## `errors.Is` vs `errors.As`

### `errors.Is(err, target)` — сравнение значений

Проходит по цепочке ошибок, ищет совпадение с конкретным значением:

```go
if errors.Is(err, sql.ErrNoRows) {
    return nil, ErrUserNotFound
}
```

Работает с обёрнутыми ошибками:

```go
wrapped := fmt.Errorf("query failed: %w", sql.ErrNoRows)
errors.Is(wrapped, sql.ErrNoRows) // true — проходит по цепочке
```

### `errors.As(err, target)` — извлечение типа

Проходит по цепочке, ищет ошибку нужного типа и извлекает данные:

```go
var pathErr *os.PathError
if errors.As(err, &pathErr) {
    fmt.Println("Path:", pathErr.Path)
    fmt.Println("Op:", pathErr.Op)
}
```

### Правило выбора

| Ситуация | Используй |
|----------|-----------|
| Проверить конкретное значение (`io.EOF`, `sql.ErrNoRows`) | `errors.Is` |
| Извлечь данные из типизированной ошибки | `errors.As` |
| Просто "ошибка есть или нет?" | `err != nil` |

**Никогда не используй `==` для сравнения ошибок** — сломается при wrapping. Всегда `errors.Is`.

---

## Паттерны на практике

### Happy path слева

```go
// ХОРОШО: успешный путь идёт ровно вниз
func processOrder(id string) error {
    order, err := fetchOrder(id)
    if err != nil {
        return fmt.Errorf("fetch order: %w", err)
    }

    if err := validateOrder(order); err != nil {
        return fmt.Errorf("validate order: %w", err)
    }

    receipt, err := chargePayment(order)
    if err != nil {
        return fmt.Errorf("charge payment: %w", err)
    }

    return sendConfirmation(order, receipt)
}
```

```go
// ПЛОХО: вложенная "лестница"
func processOrder(id string) error {
    order, err := fetchOrder(id)
    if err == nil {
        if err := validateOrder(order); err == nil {
            receipt, err := chargePayment(order)
            if err == nil {
                // глубоко вложено...
            }
        }
    }
    return err
}
```

### Паттерн errWriter (Rob Pike)

Накапливаем состояние, проверяем ошибку один раз в конце:

```go
type errWriter struct {
    w   io.Writer
    err error
}

func (ew *errWriter) write(buf []byte) {
    if ew.err != nil {
        return
    }
    _, ew.err = ew.w.Write(buf)
}

ew := &errWriter{w: fd}
ew.write(header)
ew.write(body)
ew.write(footer)
if ew.err != nil {
    return ew.err
}
```

Этот паттерн используется в `bufio.Writer`, `archive/zip`, `net/http`.

---

## Частые ошибки

### 1. Игнорирование ошибок

```go
// ПЛОХО
data, _ := json.Marshal(payload)

// ХОРОШО
data, err := json.Marshal(payload)
if err != nil {
    return fmt.Errorf("marshaling payload: %w", err)
}
```

Линтер `errcheck` ловит это.

### 2. Over-wrapping

```go
// ПЛОХО: каждый слой добавляет шум
// "calling service: calling repository: executing query: scanning row: connection refused"

// ХОРОШО: оборачивай на границах пакетов с осмысленным контекстом
return fmt.Errorf("get user %d: %w", userID, err)
```

### 3. Log AND return

```go
// ПЛОХО: одна ошибка появляется дважды в логах
if err != nil {
    log.Printf("failed: %v", err)
    return fmt.Errorf("failed: %w", err)
}

// ПРАВИЛО Dave Cheney: обрабатывай ошибку РОВНО ОДИН РАЗ
// Средний слой — возвращай:
return fmt.Errorf("connect to %s: %w", addr, err)
// Верхний слой — логируй:
log.Printf("connect to %s: %v", addr, err)
```

### 4. `log.Fatal` вне `main()`

`log.Fatal` вызывает `os.Exit(1)`, обходя все `defer`. Использовать только в `main()`.

---

## Сравнение с Python

| Аспект | Go | Python |
|--------|-----|--------|
| Видимость | Ошибки в сигнатуре `(result, error)` | Exceptions невидимы в сигнатуре |
| Flow control | Линейный, каждая ошибка на месте | Non-local jumps через стек |
| Verbosity | Больше строк кода | Меньше boilerplate |
| Забыть обработать | Возможно (`_`), но линтер ловит | Легко; необработанный exception крашит |
| Гранулярность | Per-call-site | Block-level (`try` на несколько строк) |
| Performance | Zero cost при отсутствии ошибки | Stack unwinding дорогой |

```python
# Python: чисто, но непонятно какая строка упала
try:
    user = fetch_user(id)
    order = create_order(user, items)
    receipt = process_payment(order)
except DatabaseError:
    handle_db_error()  # какой вызов упал?
```

```go
// Go: verbose, но каждая точка отказа явная
user, err := fetchUser(id)
if err != nil {
    return fmt.Errorf("fetch user %d: %w", id, err)
}
order, err := createOrder(user, items)
if err != nil {
    return fmt.Errorf("create order: %w", err)
}
```

---

## Будущее error handling в Go

Go team официально объявила (июнь 2025) что **прекращает попытки менять синтаксис**. После трёх предложений за 7 лет:

1. `check/handle` (2018) — слишком сложный
2. `try()` builtin (2019) — ~900 комментариев, сообщество против
3. `?` оператор (2024) — вдохновлён Rust, не достиг консенсуса

`if err != nil` остаётся паттерном Go навсегда. Аргумент: с опытом ощущение verbosity исчезает, а явность остаётся.

---

## Теоретические истоки

### C errno → Go `(value, error)`

Go's error model — прямой потомок **C errno** (Unix V6, 1970-е). Функции возвращают -1/NULL при ошибке + устанавливают `errno`. Go заменил глобальную переменную на **multiple return values** — чище, type-safe, thread-safe.

### FP: Either/Result монады

В функциональном программировании ту же проблему решают **sum types:**
- **ML `option`** (1980-е): `NONE | SOME of 'a`
- **Haskell `Either`**: `Left error | Right value` — формирует **монаду**, можно чейнить через `>>=`
- **Rust `Result<T, E>`**: `Ok(T) | Err(E)` + оператор `?` для раннего выхода

Go `(value, error)` — это **упрощённый Result без type-system enforcement**: оба значения независимы (оба могут быть nil или non-nil). Нет монадического chaining — каждая проверка явная.

### Почему Go отказался от монад

Rob Pike ("Errors Are Values", 2015): раз ошибки — обычные значения, можно использовать **контрольный поток языка** для их обработки (паттерн errWriter), а не полагаться на специальный синтаксис. Монады считались слишком абстрактными для целевой аудитории Go — системных программистов.

### "Worse is Better" (Richard Gabriel, 1989)

Go error handling — пример принципа "Worse is Better": интерфейс проще (нет exceptions, нет монад), ценой verbosity. Но implementation тоже проще — нет stack unwinding, нет hidden control flow.

---

## Что читать дальше

### Обязательное
- **Pike, Rob.** ["Errors Are Values."](https://go.dev/blog/errors-are-values) Go Blog, 2015
- **Go Blog.** ["Working with Errors in Go 1.13."](https://go.dev/blog/go1.13-errors) 2019
- **Go Blog.** ["On | No syntactic support for error handling."](https://go.dev/blog/error-syntax) 2025

### Углублённое
- **Wadler, P.** "Monads for Functional Programming." *Marktoberdorf Summer School*, 1992 — монадическая обработка ошибок
- **Milner, R., Tofte, M., Harper, R.** *The Definition of Standard ML.* MIT Press, 1990 — ML option type
- **Gabriel, R.P.** ["Worse is Better."](https://www.dreamsongs.com/WorseIsBetter.html) 1989

### Практическое
- **Cheney, Dave.** ["Don't just check errors, handle them gracefully."](https://dave.cheney.net/2016/04/27/dont-just-check-errors-handle-them-gracefully) 2016
- **Jay Conrod.** ["Error handling guidelines for Go."](https://jayconrod.com/posts/116/error-handling-guidelines-for-go)
