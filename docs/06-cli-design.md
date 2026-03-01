# 06 — CLI Design: Taskwarrior, Cobra, Best Practices

## Почему Taskwarrior — эталон CLI

Taskwarrior построен вокруг четырёхчастной грамматики:

```
task [filter] [command] [modifications] [miscellaneous]
```

Парсер находит **command** первым (exact match, затем abbreviation), всё до команды — **filter**, всё после — **modifications**.

```bash
task list                              # только команда
task +home list                        # фильтр + команда
task 12 modify project:Garden          # фильтр (ID) + команда + модификация
task project:Home +urgent list         # два фильтра + команда
task rc.verbose=off 12 done            # override + фильтр + команда
```

---

## Ключевые UX-паттерны Taskwarrior

### 1. Low-friction capture

Добавить задачу быстрее чем не добавить: `task add Buy milk`. Никаких флагов, структуры не нужно. Детали — потом через `modify`.

### 2. Smart defaults + progressive disclosure

Всё имеет sensible defaults. Новичок использует `task add` и `task list`. Продвинутый — фильтры, кастомные отчёты, UDA, hooks, urgency coefficients. Сложность никогда не навязывается.

### 3. Аббревиатуры команд

Minimum-unique-prefix (по дефолту 2 символа):

```bash
task li   → list
task mo   → modify
task do   → done
task de   → delete
```

### 4. Virtual tags — семантические ярлыки

Вместо сложных date expressions — 32 runtime-вычисляемых виртуальных тега:

```bash
task +TODAY list        # вместо due.after:yesterday and due.before:tomorrow
task +OVERDUE list
task +BLOCKED list
task +ACTIVE list
```

### 5. TTY-aware output

Полноцветные таблицы для терминала, stripped-вывод для pipe. `rc._forcecolor:on` для `less -R`.

### 6. Confirmation prompts

Массовые операции и удаления запрашивают подтверждение. Override: `rc.confirmation=off`.

---

## Система фильтрации Taskwarrior

### Типы фильтров

| Тип | Синтаксис | Пример |
|-----|-----------|--------|
| ID/диапазон | `12`, `1-5` | `task 1-5 list` |
| Атрибут | `name:value` | `task project:Home list` |
| Тег есть | `+tagname` | `task +urgent list` |
| Тега нет | `-tagname` | `task -work list` |
| Regex | `/pattern/` | `task /bug.*fix/ list` |
| Virtual tag | `+VIRTUAL` | `task +OVERDUE list` |

### Модификаторы атрибутов (dot-notation)

```bash
task due.before:2024-01-01 list
task urgency.above:5 list
task description.has:deploy list
task project.startswith:Home list
```

### Булевы операции

```bash
task project:Home or project:Garden list
task project:Home and +urgent list
task status:pending \( project:Home or +urgent \) list
```

---

## Конфигурация (taskrc)

### Иерархия приоритетов (высший → низший)

1. **CLI overrides:** `rc.name:value`
2. **Environment:** `TASKRC`, `TASKDATA`
3. **Config file:** `~/.taskrc`
4. **Built-in defaults**

### User Defined Attributes (UDA)

```ini
uda.difficulty.type=string
uda.difficulty.label=Diff
uda.difficulty.values=easy,medium,hard
uda.difficulty.default=medium
```

UDA работают идентично встроенным атрибутам: фильтры, сортировка, отчёты, urgency.

---

## Hooks (Git-style)

| Событие | Триггер | Может модифицировать? |
|---------|---------|----------------------|
| `on-launch` | После инициализации | Нет (но может abort) |
| `on-exit` | После обработки | Read-only |
| `on-add` | Создание задачи | Да (JSON in/out) |
| `on-modify` | Изменение задачи | Да (JSON in/out) |

Скрипты в `~/.task/hooks/`. Exit 0 = success, non-zero = abort. JSON на stdin/stdout.

---

## Cobra — паттерны для Go CLI

### Структура команд

```go
var rootCmd = &cobra.Command{Use: "wsproxy"}

var startCmd = &cobra.Command{
    Use:   "start",
    Short: "Start the proxy server",
    RunE: func(cmd *cobra.Command, args []string) error {
        // ...
    },
}

var messagesCmd = &cobra.Command{
    Use:     "messages",
    Aliases: []string{"msg"},
    Short:   "List captured messages",
}
```

### Persistent flags (наследуются подкомандами)

```go
rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
rootCmd.PersistentFlags().StringVarP(&output, "output", "o", "table", "output format")
rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
```

### Local flags (только для одной команды)

```go
startCmd.Flags().IntVarP(&port, "port", "p", 8080, "listen port")
startCmd.Flags().StringVar(&target, "target", "", "target WebSocket URL")
cobra.MarkFlagRequired(startCmd.Flags(), "target")
```

### Ключевые паттерны

- **`RunE`** (а не `Run`) — ошибки пропагируются правильно
- **`PersistentPreRunE`** — загрузка конфига, логгер, валидация для всех подкоманд
- **Args validators:** `cobra.ExactArgs(1)`, `cobra.NoArgs`
- **Flag groups:** `cmd.MarkFlagsMutuallyExclusive("json", "yaml")`

---

## Паттерны из `gh` и `kubectl`

### GitHub CLI

- `--json <fields>` с встроенными `--jq` и `--template`
- `--web` — открыть в браузере вместо CLI-вывода
- Context-awareness: автоопределение repo/branch из git
- Interactive prompts когда TTY и не хватает обязательных аргументов

### kubectl

- Multiple output formats: `-o wide`, `-o json`, `-o yaml`, `-o jsonpath='...'`
- Resource abbreviations: `po` → pods, `svc` → services
- `--watch` для live-updating output
- Label selectors: `-l app=frontend`

---

## 12-Factor CLI App

| # | Принцип | Правило |
|---|---------|--------|
| 1 | **Help** | `mycli`, `--help`, `help`, `-h` — все показывают help. Примеры первыми |
| 2 | **Flags** | Flags над args для non-trivial. Self-documenting, order-independent |
| 3 | **Stdout/Stderr** | Вывод → stdout (для pipe). Ошибки → stderr. Не смешивать |
| 4 | **Errors** | Actionable: что упало, почему, как починить |
| 5 | **TTY** | Цвет, прогресс, промпты только на TTY. Автоматически отключать в pipe |
| 6 | **Tables** | Без рамок. Каждая строка = один entry для grep/wc. Truncate до ширины терминала |
| 7 | **Speed** | Старт < 100ms. Lazy-load. Кэшировать |
| 8 | **Config** | Flags > env > project config > user config > defaults |
| 9 | **Env vars** | Префикс `WSPROXY_`. Документировать все в help |
| 10 | **Subcommands** | Группировать. Default subcommand |
| 11 | **JSON** | `--json` для machine consumption |
| 12 | **Install** | Single binary, package managers, zero runtime deps |

---

## clig.dev — дополнительные гайдлайны

- "If you change state, tell the user" — всегда подтверждать что произошло
- Секреты **никогда** через flags (видны в `ps` и shell history). Через файлы или stdin
- Suggest corrections для опечаток: "Did you mean 'start'?"
- Exit codes: 0 = success, non-zero = failure с разбивкой по категориям
- Ctrl-C: graceful exit на первое нажатие, немедленный exit на второе
- Таймауты для всех сетевых операций

---

## Применение к WebSocket-прокси

### Структура команд

```
wsproxy start --target ws://localhost:3000 --port 8080
wsproxy stop
wsproxy status

wsproxy messages                                          # default report
wsproxy messages --type text --direction inbound
wsproxy messages --since 5m --match "/error/"
wsproxy messages --json | jq '.[] | select(.size > 1024)'

wsproxy connections list
wsproxy connections inspect 3
wsproxy connections close 3

wsproxy replay session:latest
wsproxy export --format har --output capture.har
wsproxy export --format jsonl | jq '.'
```

### Конфигурация (XDG)

```toml
# ~/.config/wsproxy/config.toml
[proxy]
default_port = 8080

[output]
format = "table"
color = "auto"
columns = ["id", "direction", "type", "size", "timestamp"]

[recording]
enabled = true
path = "~/.local/share/wsproxy/sessions"
```

Override: `wsproxy --config.output.format=json start`

Env: `WSPROXY_PORT=9090 wsproxy start --target ws://...`

### Аббревиатуры и алиасы

Built-in: `msg` → messages, `conn` → connections, `sess` → sessions

### Hooks

```
~/.config/wsproxy/hooks/on-message.filter-pii
~/.config/wsproxy/hooks/on-connect.notify
~/.config/wsproxy/hooks/on-error.alert
```

JSON на stdin/out, exit 0/non-zero.

### Error messages

```
Error: Connection refused to ws://localhost:3000
  Is the target server running?
  Try: wsproxy start --target ws://localhost:3001

Error: Port 8080 is in use
  Try: wsproxy start --port 8081
```

---

## Теоретические истоки

### Unix Philosophy (McIlroy, Thompson, 1969-1978)

CLI-дизайн Taskwarrior и Go CLI-тулзов наследует **Unix philosophy:**
1. "Write programs that do one thing and do it well"
2. "Write programs to work together"
3. "Write programs to handle text streams, because that is a universal interface"

Ken Thompson (со-создатель Unix и Go) + Rob Pike (Plan 9) привнесли эту культуру напрямую в Go. `io.Reader`/`io.Writer`, stdout/stderr разделение, pipe-friendly вывод — всё оттуда.

### "Worse is Better" (Gabriel, 1989)

Go CLI-инструменты следуют принципу: простота реализации > полнота интерфейса. Cobra даёт минимум фреймворка, остальное — plain Go code. Сравни с Python Click/Typer — богаче, но тяжелее.

### Wirth: "A Plea for Lean Software" (1995)

*"People seem to misinterpret complexity as sophistication."* Taskwarrior и Go CLI tools — lean software: минимум зависимостей, single binary, instant startup.

---

## Что читать дальше

- **Raymond, E.S.** *The Art of Unix Programming.* Addison-Wesley, 2003 — [catb.org/esr/writings/taoup](http://catb.org/esr/writings/taoup/)
- **Dickey, Jeff.** ["12 Factor CLI Apps."](https://jdx.dev/posts/2018-10-08-12-factor-cli-apps/) 2018
- **clig.dev.** [Command Line Interface Guidelines](https://clig.dev/) — collective best practices
- **Taskwarrior docs.** [taskwarrior.org/docs](https://taskwarrior.org/docs/) — синтаксис, фильтры, hooks
- **Cobra docs.** [cobra.dev](https://cobra.dev/) — Go CLI framework
