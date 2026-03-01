# 07 — WebSocket Protocol & Proxy Internals

## WebSocket Handshake (HTTP Upgrade)

WebSocket переиспользует порт 80/443, стартуя поверх HTTP.

### Client Request

```
GET /chat HTTP/1.1
Host: server.example.com
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Version: 13
Sec-WebSocket-Extensions: permessage-deflate
Origin: http://example.com
```

- **`Sec-WebSocket-Key`** — случайный base64 16-байтный nonce. Не криптографический — предотвращает replay кэширующими прокси
- **`Sec-WebSocket-Version: 13`** — единственная валидная версия в RFC 6455

### Server Response

```
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=
```

- **101** — любой другой статус = handshake failed
- **`Sec-WebSocket-Accept`** = `Base64(SHA-1(Key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))`. GUID — фиксированная константа из RFC

После 101 TCP-соединение переключается на WebSocket framing. HTTP больше не используется.

---

## Формат фрейма

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-------+-+-------------+-------------------------------+
|F|R|R|R| opcode|M| Payload len |    Extended payload length    |
|I|S|S|S|  (4)  |A|     (7)     |         (16 or 64)            |
|N|V|V|V|       |S|             |                               |
| |1|2|3|       |K|             |                               |
+-+-+-+-+-------+-+-------------+-------------------------------+
|                  Masking-key (0 or 4 bytes)                   |
+---------------------------------------------------------------+
|                       Payload Data                            |
+---------------------------------------------------------------+
```

### Поля

| Поле | Биты | Описание |
|------|------|----------|
| **FIN** | 1 | `1` = последний фрагмент; `0` = ещё будут |
| **RSV1-3** | по 1 | Зарезервированы. RSV1 используется `permessage-deflate` |
| **Opcode** | 4 | Тип фрейма |
| **MASK** | 1 | `1` = payload замаскирован (client→server) |
| **Payload len** | 7 | 0-125 = длина; 126 = следующие 2 байта; 127 = следующие 8 байт |
| **Masking-key** | 0/32 | 4 байта при MASK=1 |

### Opcodes

| Opcode | Тип | Категория |
|--------|-----|-----------|
| 0 | Continuation | Data |
| **1** | **Text (UTF-8)** | **Data** |
| **2** | **Binary** | **Data** |
| 3-7 | Reserved | Data |
| **8** | **Close** | **Control** |
| **9** | **Ping** | **Control** |
| **10** | **Pong** | **Control** |
| 11-15 | Reserved | Control |

Control frames: max payload 125 байт, НЕ могут быть фрагментированы.

### Masking

Каждый client→server фрейм **обязан** быть замаскирован. Алгоритм — простой XOR:

```
transformed-octet-i = original-octet-i XOR masking-key-octet-(i MOD 4)
```

Ключ — 4 случайных байта, новый для каждого фрейма. XOR — собственная инверсия, поэтому тот же алгоритм размаскировывает.

Маскирование существует не для конфиденциальности, а для **предотвращения cache-poisoning атак** на промежуточную инфраструктуру.

---

## Фрагментация (Frame vs Message)

**Message** — прикладная единица данных. **Frame** — единица на проводе. Одно message может быть разбито на несколько frames.

### Правила

1. Первый фрейм: `FIN=0`, data opcode (1 или 2)
2. Continuation frames: `FIN=0`, opcode `0`
3. Финальный фрейм: `FIN=1`, opcode `0`

```
Frame 1: FIN=0, opcode=0x1 (text), payload="Hello "
Frame 2: FIN=0, opcode=0x0 (continuation), payload="World"
Frame 3: FIN=1, opcode=0x0 (continuation), payload="!"
=> Reassembled: "Hello World!"
```

### Control frames могут быть вставлены между фрагментами

Ping может прийти между Frame 1 и Frame 2. Control frames сами не фрагментируются.

### Импликация для прокси

**Frame-level proxy** — пересылает каждый фрейм без пересборки. Низкая latency, минимум памяти.
**Message-level proxy** — буферизует все фрагменты, пересобирает, затем может ре-фрагментировать. Нужен только если надо инспектировать/модифицировать полные сообщения.

---

## Close Handshake

1. Peer A отправляет **Close frame** (opcode 8) с optional 2-byte status code + UTF-8 reason (max 123 байта)
2. Peer B отвечает своим Close frame
3. После отправки Close — **нельзя слать data frames**
4. Инициатор закрывает TCP после получения ответного Close

### Close Status Codes

| Code | Имя | Описание |
|------|-----|----------|
| **1000** | Normal Closure | Штатное закрытие |
| **1001** | Going Away | Сервер перезапуск, навигация браузера |
| 1002 | Protocol Error | Нарушение протокола |
| 1003 | Unsupported Data | Тип данных не принимается |
| **1005** | No Status Received | **Pseudo-code** — не отправляется на wire |
| **1006** | Abnormal Closure | **Pseudo-code** — соединение упало без Close frame |
| 1007 | Invalid Payload | Non-UTF-8 в text message |
| 1009 | Message Too Big | Превышен лимит размера |
| 1011 | Internal Error | Серверная ошибка |
| 4000-4999 | Application-defined | Для приватного использования приложениями |

**Для прокси:** пересылать Close frames в обе стороны и отслеживать состояние close handshake. Если одна сторона падает без Close (code 1006) — синтезировать Close для другой стороны.

---

## Что прокси должен обработать

### Masking/Unmasking

RFC 6455:
- **Client→server frames MUST be masked**
- **Server→client frames MUST NOT be masked**

**Transparent proxy** (просто реле байтов) — не трогает маскирование. Направление сохраняется натурально.

**Intercepting proxy** (два отдельных WS-соединения) становится и "сервером" и "клиентом":
1. **Unmask** фреймы от downstream-клиента
2. **Re-mask** при пересылке upstream-серверу (новый случайный ключ)
3. **Не маскировать** при пересылке клиенту

### permessage-deflate

Сжимает payload через DEFLATE. RSV1 бит указывает на сжатое сообщение.

**Passive proxy:** пересылает extension negotiation без изменений, реле сжатых фреймов как есть. Работает только если не нужно инспектировать payload.

**Intercepting proxy — два варианта:**
1. **Strip extension** — убрать `permessage-deflate` из заголовков. Просто, но отключает компрессию
2. **Decompress/recompress** — negotiate independent на каждой ноге. Поддерживать два DEFLATE контекста

### Ping/Pong

Варианты:
- Пересылать Ping→Pong на другую сторону (transparent)
- Отвечать локально и генерировать свои Ping для liveness detection (interactive)

---

## Passive vs Interactive Proxy

### Passive (Transparent Relay)

```
Client ←ws→ Proxy ←ws→ Upstream Server
             (TCP relay)
```

- Пересылает HTTP Upgrade verbatim
- После 101 — raw TCP `io.Copy` в обе стороны
- **Не парсит** WebSocket фреймы
- Не может инспектировать, фильтровать, модифицировать
- Минимальный CPU/memory overhead

### Interactive (Intercepting / MITM)

```
Client ←ws→ Proxy (server leg)  |  Proxy (client leg) ←ws→ Upstream
             Conn A             |  Conn B
             [unmask, decode,   |  [re-mask, encode,
              decompress,       |   recompress,
              inspect/modify]   |   forward]
```

- Терминирует **два независимых** WS-соединения
- Conn A: прокси = WebSocket **server** для клиента (Upgrader)
- Conn B: прокси = WebSocket **client** для upstream (Dialer)
- Две горутины: client→server pump + server→client pump
- Полная видимость, модификация, access control

| Аспект | Passive | Interactive |
|--------|---------|-------------|
| Handshake | Пересылка verbatim | Terminate + re-initiate |
| Frame parsing | Нет | Полный |
| Masking | Не трогает | Unmask/re-mask |
| Extensions | Прозрачно | Negotiate per leg |
| Модификация | Невозможна | Полная |
| Latency | Минимальная | Выше (парсинг + ре-сериализация) |

---

## gorilla/websocket API

### Upgrader (server-side)

```go
upgrader := websocket.Upgrader{
    ReadBufferSize:  4096,
    WriteBufferSize: 4096,
    CheckOrigin:     func(r *http.Request) bool { return true },
}

conn, err := upgrader.Upgrade(w, r, nil)
```

### Dialer (client-side)

```go
conn, resp, err := websocket.DefaultDialer.Dial(targetURL, nil)
// или с context:
conn, resp, err := dialer.DialContext(ctx, targetURL, nil)
```

### Reading/Writing

**Buffer-based (простой):**

```go
msgType, payload, err := conn.ReadMessage()
err = conn.WriteMessage(msgType, payload)
```

**Streaming (для прокси — нет буферизации всего сообщения):**

```go
msgType, reader, err := conn.NextReader()
writer, err := conn.NextWriter(msgType)
io.Copy(writer, reader)
writer.Close()
```

### Control frame handlers

```go
conn.SetCloseHandler(func(code int, text string) error {
    // переслать Close на другую ногу
})

conn.SetPingHandler(func(appData string) error {
    // переслать Ping на другую ногу
})
```

### Concurrency модель

gorilla/websocket поддерживает **один concurrent reader и один concurrent writer**. Стандартный паттерн для прокси:

```go
errc := make(chan error, 2)

go func() { // client → server
    for {
        msgType, msg, err := clientConn.ReadMessage()
        if err != nil { errc <- err; return }
        err = serverConn.WriteMessage(msgType, msg)
        if err != nil { errc <- err; return }
    }
}()

go func() { // server → client
    for {
        msgType, msg, err := serverConn.ReadMessage()
        if err != nil { errc <- err; return }
        err = clientConn.WriteMessage(msgType, msg)
        if err != nil { errc <- err; return }
    }
}()

err := <-errc
```

Каждый `Conn` имеет ровно одну горутину-reader и одну горутину-writer — constraint удовлетворён.

### Полезные утилиты

```go
websocket.FormatCloseMessage(1000, "normal closure")
websocket.IsCloseError(err, websocket.CloseNormalClosure)
websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway)
websocket.IsWebSocketUpgrade(r)
```

---

## Теоретические истоки

### Finite State Machines и протоколы

WebSocket close handshake — классический **конечный автомат** (Mealy/Moore machine). `SessionState` в проекте (Created→Connecting→Active→Paused→Closing→Closed/Error) — FSM с таблицей переходов. Линтер `exhaustive` в Go заменяет формальную верификацию полноты переходов.

### MITM Proxy = Intercepting Proxy Pattern

Intercepting proxy терминирует два независимых соединения — паттерн из **"Design Patterns"** (Gamma et al., 1994): **Proxy pattern** в варианте protection/virtual proxy. Для WebSocket добавляется stateful bidirectional relay.

### Bit Manipulation — frame parsing

Парсинг WebSocket фреймов — реальный bit twiddling: FIN бит, RSV1-3, 4-bit opcode, MASK бит, variable-length payload encoding. Это closest к hardware-level programming в проекте.

---

## Что читать дальше

- **RFC 6455.** [The WebSocket Protocol](https://datatracker.ietf.org/doc/html/rfc6455) — primary spec
- **RFC 7692.** [Compression Extensions for WebSocket](https://datatracker.ietf.org/doc/html/rfc7692) — permessage-deflate
- **MDN.** [Writing WebSocket servers](https://developer.mozilla.org/en-US/docs/Web/API/WebSockets_API/Writing_WebSocket_servers) — отличный tutorial
- **gorilla/websocket.** [pkg.go.dev/github.com/gorilla/websocket](https://pkg.go.dev/github.com/gorilla/websocket) — Go library docs
