# Go Language Reference (from go.dev/ref/spec)

Quick reference ordered from most frequently needed to least.

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

**Gotcha**: `s[low:high]` shares underlying array with `s`. Mutations via one are visible in the other until `append` triggers reallocation.

---

## 2. Maps

```go
// Create
m := map[string]int{"a": 1, "b": 2}
m := make(map[string]int)
m := make(map[string]int, hint)   // capacity hint

// Read (zero value if missing)
v := m[key]
v, ok := m[key]                   // comma-ok idiom

// Write / delete
m[key] = value
delete(m, key)                    // no-op if key absent
clear(m)                          // remove all entries

// Iterate (random order each time)
for k, v := range m { }
for k := range m { }

// Length
len(m)
```

**Maps are NOT safe for concurrent use** — use `sync.Map` or protect with `sync.Mutex`.

---

## 3. Strings

```go
// Immutable sequence of bytes
s := "hello"
len(s)                            // byte count, NOT rune count
s[i]                              // byte at index (uint8)
s[1:3]                            // substring (byte slice, shares nothing)

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
A type satisfies an interface if it has all the methods.

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

**Untyped constants** have arbitrary precision and adapt to context:
```go
const x = 1 << 100               // valid (huge number)
var f float64 = x                 // used as float64
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
| `real(c)` | Real part of complex |
| `imag(c)` | Imaginary part of complex |
| `print(...)` | Low-level print to stderr |
| `println(...)` | Low-level println to stderr |

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
