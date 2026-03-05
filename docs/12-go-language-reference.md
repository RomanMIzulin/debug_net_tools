# Go Language Reference (from go.dev/ref/spec)

Quick reference ordered from most frequently needed to least.
Each section: syntax first, then tricks / gotchas / best practices.

## Table of Contents

1. [Slices](#1-slices) — create, append, copy, slice expressions, tricks, shared backing array
2. [Maps](#2-maps) — create, read, delete, set idiom, counting, grouping, nil map trap
3. [Strings](#3-strings) — immutable bytes, rune iteration, Builder, split, trim
4. [Error Handling](#4-error-handling) — wrap/unwrap, sentinel errors, custom types, Is/As
5. [Goroutines & Channels](#5-goroutines--channels) — channels, select, errgroup, fan-out, context, sync
6. [Control Flow](#6-control-flow) — for/range, if-init, switch, labels, shadow variables
7. [Structs](#7-structs) — embedding, tags, functional options, field padding
8. [Interfaces](#8-interfaces) — type assertions, type switches, nil interface trap
9. [Functions](#9-functions) — closures, defer, panic/recover, defer-in-loop trap
10. [Methods](#10-methods) — value vs pointer receiver, method expressions
11. [Type Declarations & Conversions](#11-type-declarations--conversions)
12. [Generics](#12-generics-go-118) — constraints, type inference
13. [Constants & Iota](#13-constants--iota) — bitmask, skip, stringer
14. [Operators & Precedence](#14-operators--precedence)
15. [Built-in Functions Summary](#15-built-in-functions-summary)
16. [Pointers](#16-pointers)
17. [Arrays](#17-arrays)
18. [Composite Literals](#18-composite-literals-short-forms)
19. [Package & Import](#19-package--import) — init functions
20. [Blank Identifier](#20-blank-identifier)
21. [Numeric Types Quick Ref](#21-numeric-types-quick-ref)
22. [Literal Syntax](#22-literal-syntax)
23. [Assignability & Comparability](#23-assignability--comparability)
24. [Zero Values](#24-zero-values)

**Cross-cutting:**

25. [Performance Tips](#25-performance-tips) — pre-alloc, sync.Pool, unsafe string
26. [Testing Patterns](#26-testing-patterns) — table-driven, parallel, fuzz, benchmark
27. [Useful Idioms](#27-useful-idioms) — comma-ok, Must, goroutine exit
28. [go vet / Linter Catches](#28-go-vet--linter-catches-worth-knowing)
29. [Standard Library Gems](#29-standard-library-gems) — cmp, slog, iter, timers, HTTP 1.22

---

## 1. Slices

```go
// Create
s := []int{1, 2, 3}
s := make([]T, length)
s := make([]T, length, capacity)
var s []int                       // nil slice, len=0, cap=0

// Append (ALWAYS reassign — may reallocate)
s = append(s, 4, 5)
s = append(s, other...)           // unpack another slice

// Slice expressions
s[low:high]                       // len = high - low
s[low:high:max]                   // cap = max - low
s[:]                              // full slice
s[2:]                             // from index 2
s[:3]                             // first 3

// Copy
n := copy(dst, src)               // returns number copied, min(len(dst), len(src))

// Length & capacity
len(s)
cap(s)

// Clear (zero all elements, keep length)
clear(s)
```

### Slice tricks

```go
// Pre-allocate when size is known (avoids ~10 reallocations for 1000 items)
s := make([]T, 0, expectedSize)
for ... {
    s = append(s, item)
}

// Filter in-place (no allocation)
n := 0
for _, v := range s {
    if keep(v) {
        s[n] = v
        n++
    }
}
s = s[:n]

// Remove element at index i (order preserved)
s = append(s[:i], s[i+1:]...)

// Remove element at index i (order NOT preserved, faster)
s[i] = s[len(s)-1]
s = s[:len(s)-1]

// Clone (independent copy)
clone := slices.Clone(s)          // Go 1.21+

// Sort / reverse / deduplicate
slices.Sort(s)                    // Go 1.21+, for ordered types
slices.SortFunc(s, func(a, b T) int { return cmp.Compare(a.Field, b.Field) })
slices.Reverse(s)
slices.Compact(s)                 // deduplicate sorted
slices.Insert(s, i, elem)        // insert at index

// Contains / search
slices.Contains(s, v)
slices.Index(s, v)
slices.BinarySearch(s, v)
```

### Slice gotchas

**Shared backing array**: `s[low:high]` shares underlying array with `s`. Mutations via one are visible in the other until `append` triggers reallocation.

```go
a := make([]int, 3, 5)           // len=3, cap=5
b := append(a, 4)                // len=4, cap=5 — shares array with a!
b[0] = 99                        // a[0] is also 99

c := append(a, 4, 5, 6)          // exceeds cap -> new array
c[0] = 99                        // a[0] unchanged

// Safe: force new allocation with full slice expression
b := append(a[:len(a):len(a)], 4) // cap=len, so append always allocates
```

---

## 2. Maps

```go
// Create
m := map[string]int{"a": 1, "b": 2}
m := make(map[string]int)
m := make(map[string]int, hint)   // capacity hint (reduces rehashing)

// Read (zero value if missing)
v := m[key]
v, ok := m[key]                   // comma-ok idiom

// Write / delete
m[key] = value
delete(m, key)                    // no-op if key absent
clear(m)                          // remove all entries

// Iterate (random order each time — by design)
for k, v := range m { }
for k := range m { }

// Length
len(m)
```

### Map tricks

```go
// Set (unique collection) — struct{} uses 0 bytes
seen := make(map[string]struct{})
seen[key] = struct{}{}
if _, ok := seen[key]; ok { /* exists */ }

// Counting (zero value of int is 0)
counts := make(map[string]int)
counts[word]++

// Grouping (nil slice append works)
groups := make(map[string][]Item)
groups[key] = append(groups[key], item)

// maps package (Go 1.21+)
keys := maps.Keys(m)
vals := maps.Values(m)
maps.Clone(m)                    // shallow copy
maps.Equal(m1, m2)
maps.DeleteFunc(m, pred)

// Deterministic iteration — sort keys
keys := slices.Sorted(maps.Keys(m))
for _, k := range keys { fmt.Println(k, m[k]) }
```

### Map gotchas

- **NOT safe for concurrent use** — use `sync.Map` or protect with `sync.Mutex`.
- **nil map reads OK, writes panic**:

```go
var m1 map[string]int             // nil — m1["x"] returns 0, but m1["x"] = 1 panics!
m2 := map[string]int{}           // non-nil empty — safe for reads AND writes
m3 := make(map[string]int)       // same as m2
```

---

## 3. Strings

```go
// Immutable sequence of bytes
s := "hello"
len(s)                            // byte count, NOT rune count
s[i]                              // byte at index (uint8)
s[1:3]                            // substring

// Iterate by runes
for i, r := range s { }          // i = byte offset, r = rune

// Conversions
[]byte(s)                         // string -> byte slice (copy)
string(bs)                        // byte slice -> string (copy)
[]rune(s)                         // string -> rune slice
string(rs)                        // rune slice -> string
string(65)                        // rune -> string: "A"

// Concatenation
s = "hello" + " " + "world"
```

### String tricks

```go
// Efficient concatenation (many strings) — ~3x faster than + for >5 strings
var b strings.Builder
b.Grow(estimatedSize)            // optional pre-alloc
for _, s := range parts {
    b.WriteString(s)
}
result := b.String()

// Join from slice
result := strings.Join(parts, ", ")

// Common operations (avoid regex for these)
strings.HasPrefix(s, "http://")
strings.HasSuffix(s, ".go")
strings.Contains(s, "needle")
strings.TrimSpace(s)
strings.TrimPrefix(s, "v")       // "v1.2" -> "1.2"; no-op if no prefix

// Split
parts := strings.Split("a,b,c", ",")     // ["a", "b", "c"]
parts := strings.SplitN("a,b,c", ",", 2) // ["a", "b,c"]
parts := strings.Fields("  a  b  c  ")    // ["a", "b", "c"] by whitespace

// Rune count (NOT len)
utf8.RuneCountInString(s)

// strconv is faster than fmt for simple conversions
strconv.Itoa(42)                  // faster than fmt.Sprintf("%d", 42)
strconv.FormatFloat(3.14, 'f', 2, 64)
```

---

## 4. Error Handling

```go
// error is a built-in interface
type error interface {
    Error() string
}

// Return pattern
func f() (Result, error) {
    if bad {
        return Result{}, fmt.Errorf("failed: %w", err)  // wrap with %w
    }
    return result, nil
}

// Check pattern
result, err := f()
if err != nil {
    return err                    // propagate
}

// Unwrap / inspect
errors.Is(err, target)            // check error chain
errors.As(err, &target)           // extract typed error

// Type assertion on error
if pe, ok := err.(*os.PathError); ok {
    fmt.Println(pe.Path)
}
```

### Error best practices

```go
// Sentinel errors — for errors.Is checks
var ErrNotFound = errors.New("not found")
var ErrTimeout  = errors.New("timeout")

if errors.Is(err, ErrNotFound) { /* handle */ }

// Custom error type — for errors.As + extra context
type ValidationError struct {
    Field   string
    Message string
}
func (e *ValidationError) Error() string {
    return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

var ve *ValidationError
if errors.As(err, &ve) {
    fmt.Println(ve.Field)
}

// ALWAYS add context when propagating
if err != nil {
    return fmt.Errorf("open config %s: %w", path, err)
}

// %w vs %v
fmt.Errorf("...: %w", err)       // wraps (Is/As traverse)
fmt.Errorf("...: %v", err)       // formats (chain broken, use when hiding impl details)

// Multiple errors (Go 1.20+)
err = errors.Join(err1, err2, err3)

// DON'T
if err.Error() == "not found" { }    // fragile string comparison
panic(err)                            // don't panic in library code
_ = f()                              // don't ignore errors silently
```

---

## 5. Goroutines & Channels

```go
// Launch goroutine
go f(x, y)
go func() { /* ... */ }()

// Channels
ch := make(chan T)                // unbuffered (synchronous)
ch := make(chan T, n)             // buffered (async up to n)

ch <- value                       // send (blocks if full / no receiver)
v := <-ch                         // receive (blocks if empty / no sender)
v, ok := <-ch                     // ok=false if channel closed and empty

close(ch)                         // close (only sender should close)
len(ch)                           // number of queued elements
cap(ch)                           // buffer size

// Direction-restricted types
func send(ch chan<- int) { }      // send-only
func recv(ch <-chan int) { }      // receive-only
// bidirectional assignable to directional, NOT vice versa

// Range over channel (until closed)
for v := range ch { }
```

### Select

```go
select {
case v := <-ch1:
    // received from ch1
case ch2 <- x:
    // sent to ch2
case <-ctx.Done():
    // context cancelled
default:
    // no channel ready (non-blocking)
}
```

If multiple cases ready — one is chosen **pseudo-randomly**.
Without `default` — blocks until a case is ready.

### Goroutine patterns

```go
// errgroup — run N goroutines, collect first error, cancel rest
g, ctx := errgroup.WithContext(ctx)
for _, url := range urls {
    g.Go(func() error {
        return fetch(ctx, url)
    })
}
if err := g.Wait(); err != nil { /* first error */ }

// Worker pool with semaphore
sem := make(chan struct{}, maxWorkers)
for _, task := range tasks {
    sem <- struct{}{}             // acquire slot
    go func() {
        defer func() { <-sem }()  // release slot
        process(task)
    }()
}

// Fan-out / fan-in
results := make(chan Result, len(jobs))
for _, job := range jobs {
    go func() { results <- process(job) }()
}
for range jobs {
    r := <-results                // collect
}

// Done channel (signal completion)
done := make(chan struct{})
go func() {
    defer close(done)
    // work...
}()
<-done

// Non-blocking send/receive
select {
case ch <- v:
default:                          // channel full, drop
}

// Merge channels (fan-in)
func merge[T any](channels ...<-chan T) <-chan T {
    out := make(chan T)
    var wg sync.WaitGroup
    for _, ch := range channels {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for v := range ch { out <- v }
        }()
    }
    go func() { wg.Wait(); close(out) }()
    return out
}

// Rate limiter
limiter := time.NewTicker(100 * time.Millisecond)
defer limiter.Stop()
for req := range requests {
    <-limiter.C
    go handle(req)
}
```

**Leak prevention**: every goroutine must have a guaranteed exit path. If it reads from a channel, ensure that channel will be closed or context cancelled.

### Context

```go
// ALWAYS pass context as first parameter
func DoWork(ctx context.Context, arg string) error { }

// NEVER store context in struct
type Bad struct { ctx context.Context }  // anti-pattern

// ALWAYS defer cancel
ctx, cancel := context.WithTimeout(parent, 5*time.Second)
defer cancel()

// Check cancellation in long loops
for i := range items {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }
    process(items[i])
}

// Context values (sparingly — request-scoped data only)
type ctxKey struct{}               // unexported type prevents collision
ctx = context.WithValue(ctx, ctxKey{}, value)
v := ctx.Value(ctxKey{})

// Cause (Go 1.20+)
ctx, cancel := context.WithCancelCause(parent)
cancel(fmt.Errorf("shutdown requested"))
context.Cause(ctx)                // "shutdown requested"
```

### sync Primitives

```go
// Mutex
var mu sync.Mutex
mu.Lock()
defer mu.Unlock()

// RWMutex (multiple readers, single writer)
var rw sync.RWMutex
rw.RLock(); defer rw.RUnlock()    // shared read
rw.Lock();  defer rw.Unlock()     // exclusive write

// Once (guaranteed single execution)
var once sync.Once
once.Do(func() { /* expensive init */ })

// WaitGroup
var wg sync.WaitGroup
for range 5 {
    wg.Add(1)
    go func() {
        defer wg.Done()
        work()
    }()
}
wg.Wait()

// Atomic operations (lock-free)
var counter atomic.Int64
counter.Add(1)
counter.Load()
counter.Store(0)
counter.CompareAndSwap(old, new)
```

---

## 6. Control Flow

### For

```go
for { }                           // infinite
for cond { }                      // while
for i := 0; i < n; i++ { }       // classic
for i, v := range collection { } // range

// Range works on: array, slice, string, map, channel, int (1.22+)
for i := range 10 { }            // 0..9 (Go 1.22+)
for _, v := range slice { }      // ignore index
for k := range m { }             // keys only
```

### If

```go
if x > 0 {
} else if x == 0 {
} else {
}

// Init statement (scoped to if/else chain)
if err := f(); err != nil {
    return err
}
```

### Switch

```go
// Expression switch (no fallthrough by default)
switch v {
case 1:
case 2, 3:
    fallthrough                   // explicitly fall into next case
case 4:
default:
}

// Tagless switch (if-else chain)
switch {
case x < 0:
case x == 0:
default:
}

// Type switch
switch v := x.(type) {
case int:
case string:
case nil:
default:
}
```

### Break / Continue / Labels

```go
break                             // innermost for/switch/select
continue                         // next iteration of innermost for

Outer:
for i := range rows {
    for j := range cols {
        if done { break Outer }   // break outer loop
    }
}
```

### Control flow gotchas

**Shadow variables** — `:=` in inner scope creates a NEW variable:

```go
x := 1
if true {
    x := 2                        // new x, shadows outer!
    fmt.Println(x)                // 2
}
fmt.Println(x)                    // 1 (unchanged)

// Especially dangerous with err
err := firstCall()
if err == nil {
    result, err := secondCall()   // this err is NEW, doesn't update outer
    _ = result
}
// outer err is still from firstCall
```

**Loop variable capture** (fixed in Go 1.22+, but know it):

```go
// Pre-1.22: all goroutines share same `v`
for _, v := range values {
    go func() { fmt.Println(v) }() // BUG: all print last value
}
// Fix (pre-1.22): v := v inside loop
// Go 1.22+: each iteration gets its own variable
```

---

## 7. Structs

```go
type Point struct {
    X, Y float64
}

// Literal
p := Point{X: 1, Y: 2}
p := Point{1, 2}                 // positional (fragile, avoid in public API)
pp := &Point{1, 2}               // pointer to struct

// Embedding (field promotion)
type Circle struct {
    Point                         // field name = Point; X, Y promoted
    Radius float64
}
c := Circle{Point: Point{1, 2}, Radius: 5}
c.X                               // promoted from Point

// Tags (for reflection: json, db, etc.)
type User struct {
    Name  string `json:"name" db:"user_name"`
    Email string `json:"email,omitempty"`
}

// Anonymous struct
v := struct{ X, Y int }{1, 2}
```

### Struct best practices

```go
// Functional options pattern (flexible constructors)
type Option func(*Server)

func WithPort(port int) Option {
    return func(s *Server) { s.port = port }
}
func WithTimeout(d time.Duration) Option {
    return func(s *Server) { s.timeout = d }
}

func NewServer(opts ...Option) *Server {
    s := &Server{port: 8080, timeout: 30 * time.Second}
    for _, opt := range opts {
        opt(s)
    }
    return s
}
// Usage: NewServer(WithPort(9090), WithTimeout(5*time.Second))

// Prevent unkeyed literals (force named fields in external packages)
type Config struct {
    Host string
    Port int
    _    struct{}                  // can't use Config{"localhost", 8080}
}

// Comparable structs work with ==
type Point struct { X, Y int }
p1 == p2                          // field-by-field comparison

// Method on embedded type — outer method shadows promoted one
type Base struct{}
func (Base) Greet() string { return "hello" }

type Extended struct{ Base }
func (Extended) Greet() string { return "hi" }  // shadows Base.Greet

// Struct field ordering (reduce padding)
// BAD (24 bytes):                GOOD (16 bytes):
type Bad struct {                 type Good struct {
    a bool    // 1+7 pad              b int64   // 8
    b int64   // 8                     c int32   // 4
    c int32   // 4+4 pad              a bool    // 1+3 pad
}                                 }
```

---

## 8. Interfaces

```go
type Reader interface {
    Read(p []byte) (n int, err error)
}

// Embedding
type ReadWriter interface {
    Reader
    Writer
}

// Empty interface = any type
var x any                          // any = interface{}

// Type assertion
s := x.(string)                    // panics if not string
s, ok := x.(string)               // safe: ok=false if not string

// Type switch
switch v := x.(type) {
case string:
case int:
default:
}
```

**Interfaces are satisfied implicitly** — no `implements` keyword.

### Interface best practices

```go
// Accept interfaces, return structs
func Process(r io.Reader) (*Result, error) { }  // good
func Process(r io.Reader) (io.Writer, error) { } // bad: hides concrete type

// Keep interfaces small (1-3 methods)
type Stringer interface { String() string }

// Define interfaces where they're USED, not where they're implemented
// package consumer:
type Storage interface { Get(id string) (Item, error) }
// package impl:
type SQLStorage struct { ... }    // satisfies Storage without knowing about it

// Compile-time interface satisfaction check
var _ Storage = (*SQLStorage)(nil)

// io.Reader composition (decorator pattern)
r := io.LimitReader(r, 1024)     // reads at most 1024 bytes
r = io.TeeReader(r, &buf)        // copies to buf while reading
```

### Interface gotcha: nil pointer vs nil interface

```go
type MyError struct{ msg string }
func (e *MyError) Error() string { return e.msg }

func mayFail() error {
    var e *MyError = nil
    return e                      // returns non-nil interface!
}
err := mayFail()
err == nil                        // FALSE — interface{type: *MyError, value: nil}

// Fix: return bare nil
func mayFail() error {
    if failed {
        return &MyError{"boom"}
    }
    return nil                    // bare nil, not typed nil
}
```

---

## 9. Functions

```go
// Multiple returns
func divide(a, b float64) (float64, error) { }

// Named returns (use sparingly)
func f() (n int, err error) {
    return                        // naked return: returns n, err
}

// Variadic
func sum(nums ...int) int { }
sum(1, 2, 3)
sum(slice...)                     // unpack

// First-class values
f := func(x int) int { return x * 2 }
```

### Closures

```go
func counter() func() int {
    n := 0
    return func() int {
        n++                       // captures n by reference
        return n
    }
}
```

### Defer

```go
defer f()                         // runs when enclosing function returns
defer func() { }()                // deferred closure

// LIFO order
defer fmt.Println("1")
defer fmt.Println("2")
// prints: 2, 1

// Arguments evaluated at defer statement, NOT at execution
x := 1
defer fmt.Println(x)             // prints 1 even if x changes later

// Common: cleanup
f, _ := os.Open(path)
defer f.Close()
```

### Defer gotcha: defer in loop

```go
// BAD: defers pile up until function returns
for _, f := range files {
    fd, _ := os.Open(f)
    defer fd.Close()              // won't close until function ends!
}

// FIX: wrap in closure
for _, f := range files {
    func() {
        fd, _ := os.Open(f)
        defer fd.Close()          // closes at end of each iteration
        // process fd
    }()
}
```

### Panic / Recover

```go
panic("something broke")         // unwinds stack, runs defers

defer func() {
    if r := recover(); r != nil { // catches panic
        log.Println("recovered:", r)
    }
}()
```

---

## 10. Methods

```go
type Rect struct { W, H float64 }

// Value receiver (copy)
func (r Rect) Area() float64 {
    return r.W * r.H
}

// Pointer receiver (can mutate)
func (r *Rect) Scale(factor float64) {
    r.W *= factor
    r.H *= factor
}

// Method expressions
f := Rect.Area                    // func(Rect) float64
f := (*Rect).Scale                // func(*Rect, float64)

// Method values (bound)
r := Rect{3, 4}
area := r.Area                    // func() float64 (r captured)
```

**Rule**: if any method has pointer receiver, ALL methods should have pointer receiver (consistency, interface satisfaction).

---

## 11. Type Declarations & Conversions

```go
// Type definition (new distinct type)
type Celsius float64
type Fahrenheit float64
// Celsius and Fahrenheit are different types, need explicit conversion

// Type alias (identical type, just a name)
type byte = uint8
type rune = int32
type NodeList = []*Node

// Conversion (explicit)
f := float64(42)
i := int(3.14)                   // truncates to 3
s := string([]byte{72, 105})     // "Hi"
bs := []byte("hello")
```

**Convertible**: same underlying type, or numeric-to-numeric, or string <-> []byte/[]rune.

---

## 12. Generics (Go 1.18+)

```go
// Generic function
func Map[T, U any](s []T, f func(T) U) []U {
    result := make([]U, len(s))
    for i, v := range s {
        result[i] = f(v)
    }
    return result
}

// Generic type
type Stack[T any] struct {
    items []T
}
func (s *Stack[T]) Push(v T) { s.items = append(s.items, v) }

// Constraints
[T any]                           // no constraint
[T comparable]                    // supports == and !=
[T ~int | ~float64]              // underlying type union
[T interface{ String() string }] // method constraint
[S ~[]E, E any]                  // multi-param with dependency

// Type inference (usually automatic)
result := Map(nums, double)       // T=int, U=int inferred
result := Map[int, string](nums, itoa) // explicit
```

---

## 13. Constants & Iota

```go
const Pi = 3.14159                // untyped (flexible precision)
const Max int = 100               // typed

const (
    A = iota                      // 0
    B                             // 1
    C                             // 2
)

// Bitmask pattern
const (
    Read   = 1 << iota            // 1
    Write                         // 2
    Exec                          // 4
)

// Skip values
const (
    _  = iota                     // 0 (skip)
    KB = 1 << (10 * iota)         // 1024
    MB                            // 1048576
    GB                            // 1073741824
)
```

### Constants trick

**Untyped constants** have arbitrary precision and adapt to context:
```go
const x = 1 << 100               // valid (huge number)
var f float64 = x                 // used as float64
```

**Enum with String()** — use `stringer` tool:
```go
//go:generate stringer -type=Direction
type Direction int
const (
    North Direction = iota
    South
    East
    West
)
```

---

## 14. Operators & Precedence

```
5.  *  /  %  <<  >>  &  &^       // highest
4.  +  -  |  ^
3.  ==  !=  <  <=  >  >=
2.  &&
1.  ||                            // lowest
```

```go
// Unary
-x    !b    ^i    *p    &v    <-ch

// Bitwise
x & y                             // AND
x | y                             // OR
x ^ y                             // XOR
x &^ y                            // AND NOT (bit clear)
^x                                // NOT (complement)
x << n                            // left shift
x >> n                            // right shift

// Increment/decrement are STATEMENTS, not expressions
i++                               // ok
x = i++                           // compile error
```

---

## 15. Built-in Functions Summary

| Function | Purpose |
|----------|---------|
| `make(T, ...)` | Create slice, map, or channel |
| `new(T)` | Allocate zeroed `*T` |
| `len(v)` | Length of string, array, slice, map, channel |
| `cap(v)` | Capacity of array, slice, channel |
| `append(s, ...)` | Append to slice |
| `copy(dst, src)` | Copy slice elements |
| `delete(m, k)` | Delete map entry |
| `clear(v)` | Clear map or zero slice elements |
| `close(ch)` | Close channel |
| `min(a, b, ...)` | Minimum value (Go 1.21+) |
| `max(a, b, ...)` | Maximum value (Go 1.21+) |
| `panic(v)` | Trigger panic |
| `recover()` | Catch panic (in defer only) |
| `complex(r, i)` | Construct complex number |
| `real(c)` / `imag(c)` | Real / imaginary part |
| `print` / `println` | Low-level print to stderr |

---

## 16. Pointers

```go
var p *int                        // nil pointer
x := 42
p = &x                            // address of x
*p = 100                          // dereference: x is now 100

// No pointer arithmetic in Go

// new allocates and returns pointer
p := new(int)                     // *p == 0
```

---

## 17. Arrays

```go
var a [5]int                      // zero-valued
a := [3]int{1, 2, 3}
a := [...]int{1, 2, 3}           // length inferred (3)
a := [5]int{0: 1, 4: 5}          // sparse

len(a)                            // always == cap(a) for arrays
```

**Arrays are values** — assignment copies the entire array. Use slices instead.

---

## 18. Composite Literals (Short Forms)

```go
// Nested struct/slice/map — inner type name can be omitted
m := map[string][]int{
    "a": {1, 2, 3},              // not []int{1, 2, 3}
}

pts := []Point{
    {1, 2},                       // not Point{1, 2}
    {3, 4},
}

m := map[Point]string{
    {1, 2}: "origin",            // not Point{1, 2}: "origin"
}
```

---

## 19. Package & Import

```go
package main                      // executable
package mylib                     // library

import "fmt"
import (
    "fmt"
    "os"
    mrand "math/rand"             // alias
    _ "image/png"                 // side-effect import (init only)
    . "math"                      // dot import (avoid)
)

// Exported: starts with uppercase
func PublicFunc() { }             // visible outside package
func privateFunc() { }           // package-internal
```

### Init Functions

```go
func init() { }                   // runs once at package load
// Multiple init() per file allowed, execute in source order
// Order: imported packages -> const -> var -> init()
```

---

## 20. Blank Identifier

```go
_ = expensiveComputation()        // discard value
_, err := f()                     // ignore first return
for _, v := range s { }           // ignore index

import _ "net/http/pprof"         // side-effect import

var _ Interface = (*Type)(nil)    // compile-time interface check
```

---

## 21. Numeric Types Quick Ref

```
int8     -128 to 127
int16    -32768 to 32767
int32    -2^31 to 2^31-1        (rune)
int64    -2^63 to 2^63-1
uint8    0 to 255               (byte)
uint16   0 to 65535
uint32   0 to 2^32-1
uint64   0 to 2^64-1
int      platform-dependent (32 or 64 bit)
uint     platform-dependent
float32  IEEE-754 32-bit
float64  IEEE-754 64-bit
complex64   float32 + float32
complex128  float64 + float64
uintptr  large enough to hold any pointer
```

---

## 22. Literal Syntax

```go
// Integer
42    0b1010    0o77    0xFF    1_000_000

// Float
3.14    1e6    .5    0x1.8p1

// Rune
'a'    '\n'    '\x41'    '\u0041'    '\U00000041'

// String
"interpreted\n"                   // escape sequences processed
`raw string\n`                    // literal backslash-n, can span lines

// Imaginary
3.14i    1e6i
```

---

## 23. Assignability & Comparability

**Assignable** when:
- Same type
- Identical underlying types (at least one unnamed)
- Interface satisfied
- `nil` to pointer/slice/map/channel/function/interface
- Untyped constant representable in target type

**Comparable** with `==` / `!=`:
- Booleans, numbers, strings, pointers, channels, interfaces
- Structs (if all fields comparable)
- Arrays (if element type comparable)

**NOT comparable**: slices, maps, functions (only `== nil`).

---

## 24. Zero Values

| Type | Zero Value |
|------|-----------|
| `bool` | `false` |
| `int`, `float64`, etc. | `0` |
| `string` | `""` |
| `pointer` | `nil` |
| `slice` | `nil` |
| `map` | `nil` |
| `channel` | `nil` |
| `function` | `nil` |
| `interface` | `nil` |
| `struct` | all fields zero |
| `array` | all elements zero |

---
---

# Cross-cutting: Performance, Testing, Idioms

---

## 25. Performance Tips

```go
// Pre-allocate slices
s := make([]T, 0, n)

// Pre-size maps
m := make(map[K]V, n)

// strings.Builder for concatenation (see section 3)

// sync.Pool for frequently allocated objects
var bufPool = sync.Pool{
    New: func() any { return new(bytes.Buffer) },
}
buf := bufPool.Get().(*bytes.Buffer)
buf.Reset()
// use buf
bufPool.Put(buf)

// Avoid allocations in hot paths
// BAD:  fmt.Sprintf("%s:%d", host, port)  — allocates
// GOOD: host + ":" + strconv.Itoa(port)   — less allocation

// []byte to string without copy (read-only, unsafe — only after profiling)
import "unsafe"
s := unsafe.String(&b[0], len(b))
```

---

## 26. Testing Patterns

```go
// Table-driven tests
func TestAdd(t *testing.T) {
    tests := []struct {
        name     string
        a, b     int
        expected int
    }{
        {"positive", 1, 2, 3},
        {"negative", -1, -2, -3},
        {"zero", 0, 0, 0},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := Add(tt.a, tt.b)
            if got != tt.expected {
                t.Errorf("Add(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.expected)
            }
        })
    }
}

// Parallel tests
t.Run("case", func(t *testing.T) {
    t.Parallel()
    // test body
})

// Test helpers
func setupDB(t *testing.T) *DB {
    t.Helper()                    // errors report caller's line
    db := openTestDB()
    t.Cleanup(func() { db.Close() })
    return db
}

// Temporary directory
dir := t.TempDir()                // auto-cleaned after test

// Skip slow tests
if testing.Short() { t.Skip("skipping slow test") }

// Benchmark (Go 1.24+)
func BenchmarkFoo(b *testing.B) {
    for b.Loop() { Foo() }        // replaces for i := 0; i < b.N; i++
}

// Fuzz testing (Go 1.18+)
func FuzzReverse(f *testing.F) {
    f.Add("hello")
    f.Fuzz(func(t *testing.T, s string) {
        if s != Reverse(Reverse(s)) {
            t.Errorf("double reverse mismatch")
        }
    })
}
```

---

## 27. Useful Idioms

```go
// Comma-ok for everything
v, ok := m[key]                   // map
v, ok := x.(T)                   // type assertion
v, ok := <-ch                    // channel receive

// Must pattern (panic on error — only for init/global)
var re = regexp.MustCompile(`^\d+$`)
var tmpl = template.Must(template.ParseFiles("index.html"))

// Ensure goroutine exits (ctx + select)
go func(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case item := <-work:
            handle(item)
        }
    }
}(ctx)

// Type switch on error
switch err := err.(type) {
case nil:
case *os.PathError:
    fmt.Println(err.Path)
case *net.OpError:
    fmt.Println(err.Op)
}
```

---

## 28. `go vet` / Linter Catches Worth Knowing

```go
fmt.Printf("%d", "string")       // vet: wrong format type
mu2 := mu                        // vet: copies lock value (sync.Mutex)
return x; fmt.Println("never")   // vet: unreachable code

type T struct {
    X int `json: "x"`            // vet: space after colon in tag
    Y int `json:"y" json:"z"`    // vet: duplicate tag key
}

x = x                            // vet: self-assignment
```

---

## 29. Standard Library Gems

```go
// cmp (Go 1.21+)
cmp.Compare(a, b)                 // -1, 0, 1
cmp.Or(a, b, c)                   // first non-zero value (like || for values)

// slog (Go 1.21+) — structured logging
slog.Info("request", "method", r.Method, "path", r.URL.Path)
slog.Error("failed", "err", err, "attempt", n)
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

// iter (Go 1.23+) — iterator protocol
func All[T any](s []T) iter.Seq2[int, T] { ... }

// Timers — ALWAYS stop when done
timer := time.NewTimer(5 * time.Second)
defer timer.Stop()
select {
case <-timer.C:                   // expired
case <-done:                      // cancelled
}

// Ticker
ticker := time.NewTicker(1 * time.Second)
defer ticker.Stop()
for t := range ticker.C { /* runs every second */ }

// HTTP handler patterns (Go 1.22+)
mux := http.NewServeMux()
mux.HandleFunc("GET /api/users/{id}", handler) // method + path params
```
