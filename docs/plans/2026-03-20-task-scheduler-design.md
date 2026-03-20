# Task Scheduler Design

## Goal

Hermitclaw gets a persistent SQLite-backed task scheduler with two execution modes (inject via Telegram Bot API, isolated via `claude -p`), manageable via CLI subcommands and a Claude Code skill.

## Architecture

Single `internal/scheduler` package with three files:
- `store.go` — SQLite schema, migrations, CRUD, `Store` struct
- `executor.go` — execute functions for inject and isolated modes (no interface)
- `scheduler.go` — `Scheduler` struct, polling loop, next-run computation

### Why one package

The scheduler is one cohesive feature. No `Executor` interface — the two execution modes (inject, isolated) share no behavioral contract. A `switch task.Mode` with two functions is clearer. Testing uses a function field on the `Scheduler` struct.

## Data Model

### SQLite Schema

Database location: `$XDG_DATA_HOME/hermitclaw/tasks.db` (default `~/.local/share/hermitclaw/tasks.db`). DB file created with `0600` permissions.

All timestamps are stored and compared in UTC. Go code uses `time.Now().UTC()` exclusively — never relies on SQLite `datetime('now')` defaults for scheduler-critical columns.

```sql
CREATE TABLE IF NOT EXISTS tasks (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    name           TEXT UNIQUE NOT NULL,
    schedule_type  TEXT NOT NULL CHECK (schedule_type IN ('cron', 'interval', 'once')),
    schedule       TEXT NOT NULL,
    mode           TEXT NOT NULL CHECK (mode IN ('inject', 'isolated')),
    prompt         TEXT NOT NULL,
    paused         INTEGER NOT NULL DEFAULT 0,
    next_run_at    TEXT,
    last_run_at    TEXT,
    last_status    TEXT,
    last_error     TEXT,
    created_at     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS task_runs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    started_at  TEXT NOT NULL,
    ended_at    TEXT,
    status      TEXT NOT NULL DEFAULT 'running',
    output      TEXT NOT NULL DEFAULT '',
    error       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_tasks_due ON tasks(paused, next_run_at);
CREATE INDEX IF NOT EXISTS idx_task_runs_task ON task_runs(task_id, started_at DESC);
```

### Key Design: Pre-computed `next_run_at`

The scheduler never parses cron expressions at poll time. The `next_run_at` column is computed:
- At task creation (CLI validates and computes first run)
- **Before** each execution (optimistic advance — prevents re-fire if task takes >60s)
- Recomputed on failure if the advance was wrong

The poll query is trivially cheap: `WHERE paused = 0 AND next_run_at <= ?`.

### Overlap Prevention

Before executing a due task, the scheduler updates `next_run_at` to the *next* scheduled time (optimistic advance). This prevents the same task from being picked up by the next poll tick if execution takes longer than 60 seconds. If execution fails, `next_run_at` does not need to be rolled back — the task simply runs at the next scheduled time.

### Task Struct

```go
type Task struct {
    ID           int64
    Name         string
    ScheduleType string    // "cron", "interval", "once"
    Schedule     string    // cron expr, Go duration string, or RFC3339 timestamp
    Mode         string    // "inject" or "isolated"
    Prompt       string
    Paused       bool
    NextRunAt    *time.Time
    LastRunAt    *time.Time
    LastStatus   string    // "success", "error", "" (never run)
    LastError    string    // last error message, "" on success
    CreatedAt    time.Time
}
```

### TaskRun Struct

```go
type TaskRun struct {
    ID        int64
    TaskID    int64
    StartedAt time.Time
    EndedAt   *time.Time
    Status    string // "running", "success", "error", "crashed"
    Output    string // truncated to 10KB, rune-safe
    Error     string
}
```

## Components

### Store (`internal/scheduler/store.go`)

```go
type Store struct { db *sql.DB }

func NewStore(dbPath string) (*Store, error)    // Open + WAL mode + migrate + recover stale runs
func NewMemoryStore() (*Store, error)           // For tests
func (s *Store) Close() error

// CRUD
func (s *Store) AddTask(t *Task) (int64, error)
func (s *Store) ListTasks() ([]Task, error)
func (s *Store) ResolveTask(idOrName string) (*Task, error) // Parse as int, fall back to name lookup
func (s *Store) RemoveTask(id int64) error
func (s *Store) SetPaused(id int64, paused bool) error

// Scheduling
func (s *Store) DueTasks(now time.Time) ([]Task, error)
func (s *Store) AdvanceNextRun(id int64, nextRun *time.Time) error   // Optimistic pre-execution advance
func (s *Store) UpdateAfterRun(id int64, lastRun time.Time, lastStatus, lastError string) error

// Audit
func (s *Store) InsertRun(r *TaskRun) (int64, error)
func (s *Store) CompleteRun(id int64, status, output, errMsg string) error
func (s *Store) RecentRuns(taskID int64, limit int) ([]TaskRun, error)
func (s *Store) RecoverStaleRuns() error // On startup: mark running -> crashed
```

SQLite is opened with `_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)` for concurrent access from CLI commands and the scheduler goroutine.

**On startup (`RecoverStaleRuns`):** `UPDATE task_runs SET status='crashed', ended_at=? WHERE status='running'`. This cleans up rows from tasks that were interrupted by a crash or kill.

### Execution Functions (`internal/scheduler/executor.go`)

No interface. Two plain functions + a function type for testability:

```go
// ExecuteFunc is the signature for task execution. Used for injection in tests.
type ExecuteFunc func(ctx context.Context, task *Task) (output string, err error)

func ExecuteInject(ctx context.Context, task *Task) (string, error)
func ExecuteIsolated(ctx context.Context, task *Task, claudeBin string) (string, error)
```

**ExecuteInject — full flow:**
1. Read `TELEGRAM_BOT_TOKEN` from `~/.claude/channels/telegram/.env`
2. Read `chat_id` from `~/.claude/channels/telegram/access.json` field `allowFrom[0]`
3. POST to `https://api.telegram.org/bot<token>/sendMessage` with `chat_id` and `text=task.Prompt`
4. The agent, running in the tmux session with Telegram channel active, sees this as an incoming message
5. The agent responds naturally via Telegram
6. Output returned to the scheduler is the Telegram API response (delivery confirmation only — no way to capture agent's response)

Timeout: 30-second `context.WithTimeout` on the HTTP call.

Credentials are read on each call (not cached) so credential rotation works without restart.

**ExecuteIsolated:**
1. Run `exec.CommandContext(ctx, claudeBin, "--dangerously-skip-permissions", "-p", task.Prompt)`
2. Capture combined stdout+stderr, truncated to 10KB (rune-safe via `strings.ToValidUTF8`)
3. Return output and any exec error

Timeout: 10-minute `context.WithTimeout` on the subprocess.

The `claudeBin` parameter comes from the config's `ClaudeBinary` field.

### Scheduler (`internal/scheduler/scheduler.go`)

```go
type Scheduler struct {
    store    *Store
    execFn   ExecuteFunc          // Injected; defaults to dispatch by task.Mode
    claudeBin string              // For isolated executor
    interval time.Duration        // 60s default, configurable for tests
    wg       sync.WaitGroup       // Track in-flight tasks for graceful shutdown
}

func New(store *Store, claudeBin string, interval time.Duration) *Scheduler
func (s *Scheduler) Run(ctx context.Context)     // Blocks until ctx cancelled, waits for in-flight
func (s *Scheduler) PollOnce(ctx context.Context) // Single tick, exported for tests

func ComputeNextRun(scheduleType, schedule string, from time.Time) (*time.Time, error)
```

The default `execFn` dispatches by `task.Mode`:
```go
func (s *Scheduler) defaultExecFn(ctx context.Context, task *Task) (string, error) {
    switch task.Mode {
    case "inject":
        return ExecuteInject(ctx, task)
    case "isolated":
        return ExecuteIsolated(ctx, task, s.claudeBin)
    default:
        return "", fmt.Errorf("unknown mode: %s", task.Mode)
    }
}
```

Tests inject a mock `execFn` that records calls.

**Poll logic (sequential execution):**
1. `store.DueTasks(time.Now().UTC())` — returns tasks where `paused = 0 AND next_run_at <= now`
2. For each due task, **sequentially** (not parallel):
   a. Compute next run time via `ComputeNextRun`
   b. `store.AdvanceNextRun(task.ID, nextRun)` — prevents re-fire on next poll
   c. For "once" tasks: `store.SetPaused(task.ID, true)` — pause regardless of outcome
   d. `store.InsertRun(...)` with `status='running'`
   e. Call `s.execFn(ctx, task)` with appropriate timeout
   f. `store.CompleteRun(runID, status, output, error)`
   g. `store.UpdateAfterRun(task.ID, now, status, error)`

Sequential execution avoids unbounded goroutine spawning after downtime and eliminates concurrency concerns. The 60-second poll interval is coarse enough that parallelism adds no meaningful benefit.

**ComputeNextRun logic:**
- `cron`: `gronx.NextTickAfter(expr, from, false)` — next occurrence after `from`
- `interval`: `from.Add(parsedDuration)` — anchored to execution time, not wall clock
- `once`: returns nil (task is already paused before execution)

**On startup:**
1. `store.RecoverStaleRuns()` — mark interrupted runs as crashed
2. Scan all active tasks with nil `next_run_at`, compute from `created_at`
3. Tasks with `next_run_at` in the past are immediately due on first poll

**Graceful shutdown:** When `ctx` is cancelled, the poll loop exits. Since execution is sequential within the loop, in-flight work is just the current task. The `Run` method returns after the current task completes (or its timeout fires).

## Integration Points

### `_run` command (`main.go`)

Before the restart loop, the `_run` command:
1. Opens the store: `scheduler.NewStore(dataDir() + "/tasks.db")`
2. Creates the scheduler: `scheduler.New(store, cfg.ClaudeBinary, 60*time.Second)`
3. Starts the scheduler in a goroutine: `go sched.Run(ctx)`
4. Defers cancel + store close

The scheduler runs independently of the Claude restart loop. Tasks fire even during restarts.

```go
func dataDir() string {
    dir := os.Getenv("XDG_DATA_HOME")
    if dir == "" {
        home, _ := os.UserHomeDir()
        dir = filepath.Join(home, ".local", "share")
    }
    return filepath.Join(dir, "hermitclaw")
}
```

### CLI commands (`main.go`)

New `schedule` command group with subcommands:

```
hermitclaw schedule add     --name NAME --cron|--interval|--once EXPR --mode inject|isolated --prompt "..."
hermitclaw schedule list
hermitclaw schedule remove  ID|NAME
hermitclaw schedule pause   ID|NAME
hermitclaw schedule resume  ID|NAME
```

No `history` subcommand (YAGNI — add later if needed). Run history is in the DB for debugging via `sqlite3`.

All CLI commands open their own short-lived SQLite connection. No IPC needed — the scheduler picks up changes on next poll (at most 60 seconds).

**`add` validation:**
- Exactly one of `--cron`, `--interval`, `--once` required
- Cron validated via `gronx.New().IsValid(expr)`
- Interval validated via `time.ParseDuration(schedule)`
- Once validated via `time.Parse(time.RFC3339, schedule)`
- Name uniqueness enforced by SQL `UNIQUE` constraint
- Prompt must be non-empty and contain non-whitespace

**`list` output:**
```
ID  Name             Type      Schedule        Mode      Status  Next Run             Last Run
1   daily-summary    cron      0 9 * * *       inject    active  2026-03-21 09:00:00  2026-03-20 09:00:00
2   weekly-cleanup   interval  168h            isolated  active  2026-03-24 09:00:00  2026-03-17 09:00:00
3   one-shot-report  once      2026-03-21T10…  isolated  paused  -                    2026-03-21 10:00:00
```

**`remove`/`pause`/`resume`**: Accept either numeric ID or task name via `store.ResolveTask()`.

### Nix module (`flake.nix`)

- Update `vendorHash` after adding dependencies
- Register the schedule skill: `skills.schedule = lib.mkDefault ./skills/schedule;`
- No scheduler config options in the module (tasks are runtime data, not declarative config)

### Skill (`skills/schedule/SKILL.md`)

Teaches the agent how to use `hermitclaw schedule` commands. Covers:
- All subcommands with examples
- Mode explanations (inject sends a Telegram message the agent sees; isolated spawns a separate claude)
- Schedule type explanations (cron vs interval vs once)
- Notes about 60-second poll granularity
- Notes about once-tasks pausing after execution (resume to retry)

## Dependencies

| Library | Import | Purpose |
|---------|--------|---------|
| `modernc.org/sqlite` | `_ "modernc.org/sqlite"` | Pure Go SQLite (no CGO) |
| `github.com/adhocore/gronx` | `github.com/adhocore/gronx` | Cron expression parsing + next-tick computation |

Both are pure Go. `CGO_ENABLED=0` build continues to work. Binary size increases ~15MB from the SQLite transpilation. Initial builds may take 2-3 minutes due to transpiled C code.

The poll interval (60s) is deliberately not user-configurable — it's only injectable for tests. This is not configuration, it's an implementation detail.

## Error Handling

- **Scheduler startup failure is non-fatal.** If SQLite can't open, the restart loop still runs. Error logged to stderr.
- **Individual task failures are recorded, not propagated.** Each run gets a `task_runs` row and the task's `last_status`/`last_error` are updated. The scheduler never crashes from a task failure.
- **Missing Telegram credentials:** `ExecuteInject` returns a clear error ("telegram not configured: ~/.claude/channels/telegram/.env not found"). Recorded in task's `last_error`. Task stays active for cron/interval — retries on next schedule. For once-tasks, the task is paused with the error recorded.
- **Missing claude binary:** `ExecuteIsolated` returns the exec error. Same behavior.
- **Execution timeout:** Inject: 30 seconds. Isolated: 10 minutes. Enforced via `context.WithTimeout`. Timeout errors recorded like any other failure.

## Security

- No secrets in the database. Bot token read from filesystem at execution time.
- DB file created with `0600` permissions.
- `--dangerously-skip-permissions` on isolated claude calls is consistent with hermitclaw's design.
- Telegram credential paths (`~/.claude/channels/telegram/`) are Claude Code internals and may change. Noted as a fragility — no abstraction, just a comment in the code.

## Testing Strategy

- **Store tests:** `NewMemoryStore()` with in-memory SQLite. Test CRUD, `DueTasks`, `AdvanceNextRun`, `RecoverStaleRuns`, run tracking.
- **Scheduler tests:** Real in-memory store + mock `execFn`. `PollOnce()` is the testable unit — call directly, avoid goroutine timing. Test all three schedule types, overlap prevention via advance, error paths, once-task auto-pause, stale run recovery.
- **Executor tests:** `ExecuteIsolated` tested with `echo` as the claude binary. `ExecuteInject` tested with `httptest.NewServer` faking the Telegram API.
- **Integration test:** Wire real in-memory store + mock execFn, create a task due in the past, call `PollOnce`, assert executor called, run recorded, and `next_run_at` advanced.

## Files Changed

| File | Action | Description |
|------|--------|-------------|
| `internal/scheduler/store.go` | CREATE | SQLite schema, Store struct, CRUD, queries |
| `internal/scheduler/executor.go` | CREATE | ExecuteFunc type, ExecuteInject, ExecuteIsolated |
| `internal/scheduler/scheduler.go` | CREATE | Scheduler struct, poll loop, ComputeNextRun |
| `internal/scheduler/scheduler_test.go` | CREATE | All tests |
| `skills/schedule/SKILL.md` | CREATE | Agent skill for managing tasks |
| `main.go` | MODIFY | Add scheduleCmd group, wire scheduler into _run, add dataDir() |
| `go.mod` | MODIFY | Add modernc.org/sqlite, adhocore/gronx |
| `flake.nix` | MODIFY | Update vendorHash, register schedule skill |
| `install.sh` | MODIFY | Install schedule + self-management skills |
| `scripts/check.sh` | MODIFY | Fix stale hooks/welcome-telegram path (separate commit) |
| `CLAUDE.md` | MODIFY | Add scheduler to architecture section, fix stale TOML parsing claim |

---

## Review Notes

Reviewed: 2026-03-20
Cycles: 2 (5 reviewers per cycle)

### Cycle 1: 3 FAIL, 2 PASS

Key issues found and resolved:
- **Executor interface unnecessary** → replaced with `ExecuteFunc` type + plain functions
- **No overlap protection** → optimistic `next_run_at` advance before execution
- **Unbounded goroutines** → sequential execution per poll tick
- **No execution timeout** → 30s inject, 10min isolated via `context.WithTimeout`
- **UTC consistency** → all timestamps from Go UTC, no SQLite defaults
- **Stale runs after crash** → `RecoverStaleRuns` on startup
- **Once-task infinite retry** → pause before execution regardless of outcome
- **Inject mode under-specified** → full 6-step flow documented
- **task_runs overbuilt** → kept but slimmed (no history command, no CleanupOldRuns)

### Cycle 2: 5 PASS

Remaining issues noted for implementation:
- Remove `sync.WaitGroup`, use `done chan struct{}` for Run exit signaling
- `NewStore` must `os.MkdirAll` parent directory before opening DB
- `SetPaused(id, false)` for once-tasks must also set `next_run_at = now` (otherwise resumed once-tasks are zombies)
- `RecoverStaleRuns` should also update `tasks.last_status` / `last_error`
- Timeouts owned by executor functions, not scheduler
- Use `"--"` separator in `ExecuteIsolated` before prompt argument
- Use `filepath.Join` for DB path construction
- Output truncation (10KB, rune-safe) at single point before `CompleteRun`
- tmux kill = ungraceful shutdown; `RecoverStaleRuns` is the expected recovery path, no signal handler needed
- Catch-up after downtime: each task fires once, `ComputeNextRun` from execution time puts next run in the future (document explicitly)
- Once-task `--once` values converted to UTC via `.UTC()` after parsing
