# 03 — Interfaces and Composition в Go

## Implicit Interface Satisfaction

Go не имеет ключевого слова `implements`. Тип удовлетворяет интерфейсу просто имея все нужные методы — **structural typing** на этапе компиляции.

```go
type Speaker interface {
    Speak() string
}

type Dog struct{}

func (d Dog) Speak() string { return "Woof" }

// Dog удовлетворяет Speaker автоматически
var s Speaker = Dog{}
```

**Три причины такого дизайна:**

1. **Развязка** — реализующий тип не импортирует пакет с интерфейсом. Нет циклических зависимостей
2. **Ретроактивное удовлетворение** — можно определить интерфейс, который уже существующие типы удовлетворяют без их модификации
3. **Нет церемоний** — нет boilerplate `implements` декларации

**Отличие от Python duck typing:** Go проверяет это **на этапе компиляции**. Если `Dog` не имеет `Speak()`, код не скомпилируется. Python Protocol (PEP 544) проверяет только если запустить mypy.

**Compile-time guard (необязательный):**

```go
var _ Speaker = (*Dog)(nil) // не скомпилируется если Dog не удовлетворяет Speaker
```

---

## Принцип маленьких интерфейсов

> *"The bigger the interface, the weaker the abstraction."* — Rob Pike

Go идиоматично использует интерфейсы с 1-2 методами:

| Интерфейс | Методы | Пакет |
|-----------|--------|-------|
| `io.Reader` | `Read(p []byte) (n int, err error)` | io |
| `io.Writer` | `Write(p []byte) (n int, err error)` | io |
| `io.Closer` | `Close() error` | io |
| `fmt.Stringer` | `String() string` | fmt |
| `error` | `Error() string` | builtin |
| `http.Handler` | `ServeHTTP(ResponseWriter, *Request)` | net/http |

**Почему:**
- Высокая вероятность множества реализаций. `io.Reader` удовлетворяют файлы, сокеты, буферы, gzip, HTTP body, strings.Reader...
- Легче тестировать — 1-методный интерфейс = 1-методный fake
- Composability — маленькие интерфейсы комбинируются через embedding

**Антипаттерн:**

```go
// ПЛОХО: kitchen-sink интерфейс
type UserService interface {
    GetUser(id int) (*User, error)
    ListUsers() ([]*User, error)
    CreateUser(u *User) error
    UpdateUser(u *User) error
    DeleteUser(id int) error
    SendEmail(to, subject, body string) error
    GenerateReport() ([]byte, error)
}
```

Лучше разбить на role-specific интерфейсы на стороне потребителя.

---

## "Accept interfaces, return structs"

Один из важнейших принципов API-дизайна в Go.

**Принимай интерфейсы** (в параметрах):
- Функция гибкая: подходит любой тип, удовлетворяющий интерфейс
- Тестируемость: можно передать test double

**Возвращай структуры** (из функций):
- Вызывающий видит все поля и методы
- Может хранить как интерфейс или как конкретный тип — на его выбор

```go
// Принимаем интерфейс
func ProcessData(r io.Reader) error {
    data, err := io.ReadAll(r)
    // ...
}

// Возвращаем конкретную структуру
func NewUserStore(db *sql.DB) *PostgresUserStore {
    return &PostgresUserStore{db: db}
}

// Вызывающий может использовать как интерфейс
var store UserStore = NewUserStore(db)
```

**Когда нарушать:** `errors.New()` возвращает `error` (интерфейс), потому что конкретный тип намеренно скрыт.

---

## Interface Embedding

Композиция больших интерфейсов из маленьких:

```go
type Reader interface {
    Read(p []byte) (n int, err error)
}

type Writer interface {
    Write(p []byte) (n int, err error)
}

type ReadWriter interface {
    Reader
    Writer
}
```

Тип удовлетворяет `ReadWriter` если удовлетворяет и `Reader`, и `Writer`. Можно встраивать интерфейсы из разных пакетов:

```go
type MyInterface interface {
    io.Reader
    fmt.Stringer
}
```

---

## Struct Embedding vs Named Fields

Go не имеет наследования. Есть **embedding** — автоматическое делегирование.

### Embedding (анонимное поле):

```go
type Animal struct {
    Name string
}

func (a Animal) Speak() string {
    return a.Name + " speaks"
}

type Dog struct {
    Animal       // embedded
    Breed string
}

d := Dog{Animal: Animal{Name: "Rex"}, Breed: "Labrador"}
d.Speak() // "Rex speaks" — промотировано из Animal
d.Name    // "Rex" — поля тоже промотируются
```

### Критическое отличие от наследования

Когда `Animal.Speak()` вызывается через `Dog`, receiver — это `Animal`, а не `Dog`. Нет полиморфного `self`/`this`:

```go
func (a Animal) WhoAmI() string {
    return fmt.Sprintf("I am %T", a) // ВСЕГДА "Animal", никогда "Dog"
}
```

### Named field (явная композиция):

```go
type Dog struct {
    beast Animal // именованное поле
    Breed string
}

// Нет промоутинга: делегируем вручную
func (d Dog) Speak() string {
    return d.beast.Speak()
}
```

### Когда что:
- **Embedding** — когда хочешь "наследовать" method set и удовлетворять те же интерфейсы
- **Named field** — когда внутренний тип — деталь реализации

Пример из stdlib: `bufio.ReadWriter` встраивает `*bufio.Reader` и `*bufio.Writer`, автоматически удовлетворяя `io.ReadWriter`.

---

## `any` (interface{})

С Go 1.18, `any` — алиас для `interface{}`. Означает "любой тип".

**Когда допустимо:**
- Функции, работающие с любым типом: `fmt.Println(a ...any)`, `json.Unmarshal`
- Границы сериализации (JSON, БД)

**Когда НЕ допустимо:**
- Когда тип известен — используй generics или конкретный интерфейс
- Злоупотребление `any` убивает статическую проверку типов

**Type assertions:**

```go
s, ok := v.(string) // безопасная форма
n := v.(int)        // паника если не int
```

**Type switches:**

```go
switch x := v.(type) {
case string:
    return "string: " + x
case int:
    return "int: " + strconv.Itoa(x)
default:
    return fmt.Sprintf("unknown: %T", x)
}
```

**Post-generics (Go 1.18+):** для обобщённых алгоритмов (`Max`, `Min`, `Contains`) используй `[T comparable]` или `[T constraints.Ordered]`, а не `any`.

---

## Практический пример: Store за интерфейсом

Это напрямую применимо к `internal/storage` нашего проекта.

### Определяем интерфейс на стороне потребителя:

```go
// Только методы, которые этому потребителю нужны
type SessionGetter interface {
    GetSession(ctx context.Context, id uuid.UUID) (*core.Session, error)
}

type SessionLister interface {
    ListSessions(ctx context.Context) ([]*core.Session, error)
}
```

### Реализация:

```go
type SQLiteStore struct {
    db *sql.DB
}

func (s *SQLiteStore) GetSession(ctx context.Context, id uuid.UUID) (*core.Session, error) {
    // squirrel query...
}

func (s *SQLiteStore) ListSessions(ctx context.Context) ([]*core.Session, error) {
    // squirrel query...
}
```

### Тестовый дубль:

```go
type fakeStore struct {
    sessions map[uuid.UUID]*core.Session
    err      error
}

func (f *fakeStore) GetSession(_ context.Context, id uuid.UUID) (*core.Session, error) {
    if f.err != nil {
        return nil, f.err
    }
    s, ok := f.sessions[id]
    if !ok {
        return nil, storage.ErrNotFound
    }
    return s, nil
}
```

Нет фреймворка, нет reflection, нет code generation. Простой struct с нужными методами.

---

## Сравнение с Python

| Аспект | Go Interfaces | Python Protocols (PEP 544) | Python ABCs |
|--------|--------------|---------------------------|-------------|
| Типизация | Structural (compile-time) | Structural (mypy) | Nominal (`class Foo(ABC)`) |
| Явная декларация | Не нужна | Не нужна | Обязательна |
| Когда проверяется | Всегда при компиляции | Опционально (mypy/pyright) | В runtime |
| Enforcement | Обязательно | Только с тулами | TypeError в runtime |

Python Protocol — ближайший аналог Go interfaces. Оба используют structural subtyping. Но Go проверяет безусловно при компиляции, а Python — только если запустить mypy.

```python
from typing import Protocol

class Speaker(Protocol):
    def speak(self) -> str: ...

class Dog:
    def speak(self) -> str:
        return "Woof"

def greet(s: Speaker) -> None:  # mypy проверяет
    print(s.speak())
```

```go
type Speaker interface {
    Speak() string
}

type Dog struct{}
func (d Dog) Speak() string { return "Woof" }

func Greet(s Speaker) {  // компилятор проверяет
    fmt.Println(s.Speak())
}
```

---

## Теоретические истоки

### Structural Typing (Cardelli & Wegner, 1985)

Go интерфейсы — **structural subtyping** на этапе компиляции. Теоретическая база:
- **Cardelli, L. & Wegner, P.** "On Understanding Types, Data Abstraction, and Polymorphism." *ACM Computing Surveys* 17(4), 1985 — фундаментальная таксономия систем типов
- **Wand, M.** "Complete Type Inference for Simple Objects." *LICS*, 1987 — **row polymorphism**, типирование записей по структуре
- **OCaml** реализует structural object types через row variables — Go проще (нет row variables), но принцип тот же

В **nominal typing** (Java/C#) типы совместимы только через `implements`/`extends`. В structural typing — через совпадение структуры (методов).

### Composition over Inheritance (GoF, 1994)

Go embedding — реализация принципа из **"Design Patterns"** (Gamma, Helm, Johnson, Vlissides, 1994): *"Favor object composition over class inheritance"*.

Предшественники:
- **Lieberman, H.** "Using Prototypical Objects..." *OOPSLA*, 1986 — ввёл термин **delegation** в ОО-программировании
- **Schärli, N. et al.** "Traits: Composable Units of Behaviour." *ECOOP*, 2003 — traits как альтернатива множественному наследованию. Влияние на Scala traits, Rust traits

Go embedding — это delegation через forwarding: методы embedded типа промотируются, но нет subtyping relationship. Проще чем traits (нет conflict resolution rules), но решает ту же проблему.

### ISP — Interface Segregation Principle (Martin, 2002)

Go маленькие интерфейсы — чистейшее выражение ISP из **SOLID** (Robert C. Martin, *Agile Software Development*, 2002): *"Clients should not be forced to depend on methods they do not use."*

Dave Cheney (GolangUK, 2016): "Well designed interfaces are more likely to be small interfaces; the prevailing idiom is an interface contains only a single method."

Structural typing делает ISP path of least resistance — большие интерфейсы тяжело удовлетворить случайно.

### Encapsulation: пакеты вместо классов (Wirth → Go)

Go's package-level encapsulation (uppercase = exported) — из **Oberon** (Wirth, 1987), где export обозначался `*`. Robert Griesemer (PhD at ETH Zurich under Wirth) привнёс напрямую.

Отличие от **class-level encapsulation** (Java `private`): в Go весь код в пакете видит всё друг друга. Это поощряет организацию по concern/module, а не по классам.

---

## Что читать дальше

### Теория типов
- **Cardelli, L. & Wegner, P.** "On Understanding Types, Data Abstraction, and Polymorphism." *ACM Comp. Surveys*, 1985
- **Wadler, P. & Blott, S.** "How to make ad-hoc polymorphism less ad hoc." *POPL*, 1989 — typeclasses (Haskell), сравни с Go interfaces

### Composition и паттерны
- **Gamma, E. et al.** *Design Patterns.* Addison-Wesley, 1994 — "Favor composition over inheritance"
- **Schärli, N. et al.** "Traits: Composable Units of Behaviour." *ECOOP*, 2003
- **Norvig, P.** ["Design Patterns in Dynamic Languages."](https://norvig.com/design-patterns/) 1996 — 16 из 23 GoF паттернов упрощаются

### Go-specific
- **Cheney, Dave.** ["SOLID Go Design."](https://dave.cheney.net/2016/08/20/solid-go-design) GolangUK, 2016
- **Griesemer, R.** ["The Evolution of Go."](https://talks.golang.org/2015/gophercon-goevolution.slide) GopherCon, 2015
- **Wirth, N.** *Programming in Oberon.* Addison-Wesley, 1992
