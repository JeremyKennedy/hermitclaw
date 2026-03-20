# Task Scheduler — Implementation Plan

**Goal:** Add a persistent SQLite-backed task scheduler to hermitclaw with inject (Telegram) and isolated (`claude -p`) execution modes.

**Architecture:** Single `internal/scheduler` package (store.go, executor.go, scheduler.go). No interfaces — plain functions + `ExecuteFunc` type. Sequential execution.

**Tech Stack:** Go 1.25, modernc.org/sqlite (pure Go), adhocore/gronx (cron parsing), cobra (CLI).

**Design Doc:** `docs/plans/2026-03-20-task-scheduler-design.md`

---

## Pre-work

### Task 0: Fix scripts/check.sh stale path
- [ ] Read `scripts/check.sh`
- [ ] Replace `hooks/welcome-telegram` reference with `skills/welcome/SKILL.md`
- [ ] Add checks for `skills/self-management/SKILL.md` and `skills/schedule/SKILL.md` (the last will exist by end of this plan)
- [ ] Commit: `fix: update check.sh structure validation for skills directory`

### Task 1: Add dependencies
- [ ] `cd /home/jeremy/dev/hermitclaw`
- [ ] `go get modernc.org/sqlite@latest`
- [ ] `go get github.com/adhocore/gronx@latest`
- [ ] `go mod tidy && go mod vendor`
- [ ] Verify: `CGO_ENABLED=0 go build -o /dev/null .` succeeds
- [ ] Commit: `chore: add modernc.org/sqlite and adhocore/gronx dependencies`

---

## Phase 1: Store

### Task 2: Create store.go — schema and Store struct
- [ ] Create `internal/scheduler/store.go`
- [ ] Define `Task` struct (ID, Name, ScheduleType, Schedule, Mode, Prompt, Paused, NextRunAt, LastRunAt, LastStatus, LastError, CreatedAt)
- [ ] Define `TaskRun` struct (ID, TaskID, StartedAt, EndedAt, Status, Output, Error)
- [ ] Define `Store` struct wrapping `*sql.DB`
- [ ] Implement `NewStore(dbPath string)`: `os.MkdirAll` parent dir, open with WAL + busy_timeout + foreign_keys, `db.SetMaxOpenConns(1)`, call `migrate()`
- [ ] Implement `NewMemoryStore()`: open `:memory:`, call `migrate()`
- [ ] Implement `migrate()`: `CREATE TABLE IF NOT EXISTS` for both tables + indexes
- [ ] Implement `Close()`

### Task 3: Store CRUD methods
- [ ] `AddTask(t *Task) (int64, error)` — validate non-empty prompt, INSERT with all fields set from Go (UTC timestamps), return ID
- [ ] `ListTasks() ([]Task, error)` — SELECT all ordered by created_at
- [ ] `ResolveTask(idOrName string) (*Task, error)` — try `strconv.ParseInt`, fall back to name lookup
- [ ] `RemoveTask(id int64) error` — DELETE (cascade deletes task_runs)
- [ ] `SetPaused(id int64, paused bool) error` — UPDATE paused. If unpausing a once-task, also set `next_run_at = time.Now().UTC()`

### Task 4: Store scheduling methods
- [ ] `DueTasks(now time.Time) ([]Task, error)` — `WHERE paused = 0 AND next_run_at IS NOT NULL AND next_run_at <= ?`
- [ ] `AdvanceNextRun(id int64, nextRun *time.Time) error` — UPDATE next_run_at
- [ ] `UpdateAfterRun(id int64, lastRun time.Time, lastStatus, lastError string) error` — UPDATE last_run_at, last_status, last_error

### Task 5: Store audit methods
- [ ] `InsertRun(r *TaskRun) (int64, error)` — INSERT with started_at from Go UTC
- [ ] `CompleteRun(id int64, status, output, errMsg string) error` — UPDATE ended_at, status, output, error
- [ ] `RecentRuns(taskID int64, limit int) ([]TaskRun, error)` — SELECT ordered by started_at DESC
- [ ] `RecoverStaleRuns() error` — `UPDATE task_runs SET status='crashed', ended_at=? WHERE status='running'` AND `UPDATE tasks SET last_status='crashed', last_error='interrupted by process exit' WHERE id IN (...)`

### Task 6: Store tests
- [ ] Write `internal/scheduler/store_test.go`
- [ ] Test `NewMemoryStore` + `Close`
- [ ] Test `AddTask` + `ListTasks` (verify all fields roundtrip)
- [ ] Test `ResolveTask` by ID and by name
- [ ] Test `RemoveTask` (verify cascade deletes runs)
- [ ] Test `SetPaused` (pause + resume, verify once-task resume sets next_run_at)
- [ ] Test `DueTasks` (due vs not-due vs paused)
- [ ] Test `AdvanceNextRun` + `UpdateAfterRun`
- [ ] Test `InsertRun` + `CompleteRun` + `RecentRuns`
- [ ] Test `RecoverStaleRuns` (verify both task_runs and tasks updated)
- [ ] Verify: `CGO_ENABLED=0 go test ./internal/scheduler/...` passes
- [ ] Commit: `feat: add scheduler store with SQLite persistence`

---

## Phase 2: Executors

### Task 7: Create executor.go
- [ ] Create `internal/scheduler/executor.go`
- [ ] Define `ExecuteFunc` type: `func(ctx context.Context, task *Task) (output string, err error)`
- [ ] Implement `ExecuteInject(ctx context.Context, task *Task) (string, error)`:
  - Read bot token from `~/.claude/channels/telegram/.env` (parse `TELEGRAM_BOT_TOKEN=...`)
  - Read chat_id from `~/.claude/channels/telegram/access.json` (`allowFrom[0]`)
  - Create `context.WithTimeout(ctx, 30*time.Second)`
  - POST to `https://api.telegram.org/bot<token>/sendMessage` with form data
  - Return response body or error
- [ ] Implement `ExecuteIsolated(ctx context.Context, task *Task, claudeBin string) (string, error)`:
  - Create `context.WithTimeout(ctx, 10*time.Minute)`
  - Run `exec.CommandContext(ctx, claudeBin, "--dangerously-skip-permissions", "-p", "--", task.Prompt)`
  - Capture combined output, truncate to 10KB (rune-safe: `strings.ToValidUTF8`)
  - Return output and error

### Task 8: Executor tests
- [ ] Test `ExecuteIsolated` with `echo` as claude binary — verify output captured
- [ ] Test `ExecuteIsolated` timeout with `sleep` as claude binary
- [ ] Test `ExecuteInject` with `httptest.NewServer` faking Telegram API — verify correct POST body
- [ ] Test `ExecuteInject` with missing credential files — verify clear error message
- [ ] Commit: `feat: add scheduler executor functions (inject + isolated)`

---

## Phase 3: Scheduler

### Task 9: Create scheduler.go
- [ ] Create `internal/scheduler/scheduler.go`
- [ ] Implement `ComputeNextRun(scheduleType, schedule string, from time.Time) (*time.Time, error)`:
  - `cron`: `gronx.NextTickAfter(expr, from, false)` — convert to `*time.Time`
  - `interval`: `time.ParseDuration(schedule)`, return `from.Add(d)`
  - `once`: return nil
- [ ] Define `Scheduler` struct: store, execFn, claudeBin, interval, done chan
- [ ] Implement `New(store, claudeBin, interval)` — set default execFn dispatch
- [ ] Implement `defaultExecFn` — switch on task.Mode, call ExecuteInject or ExecuteIsolated
- [ ] Implement `Run(ctx context.Context)`:
  - Call `store.RecoverStaleRuns()`
  - Initialize nil `next_run_at` for active tasks
  - Ticker loop with `select` on `ctx.Done()` and `ticker.C`
  - Call `PollOnce` on each tick (and once immediately on start)
  - Close `done` channel on exit
- [ ] Implement `Wait()` — blocks on `done` channel (for shutdown coordination)
- [ ] Implement `PollOnce(ctx context.Context)`:
  - `store.DueTasks(time.Now().UTC())`
  - For each task sequentially:
    - `ComputeNextRun` from now
    - `store.AdvanceNextRun(id, nextRun)` — overlap prevention
    - If once-task: `store.SetPaused(id, true)`
    - `store.InsertRun(...)` with status=running
    - Call `s.execFn(ctx, task)`
    - Truncate output to 10KB rune-safe
    - `store.CompleteRun(runID, status, output, error)`
    - `store.UpdateAfterRun(id, now, status, error)`

### Task 10: Scheduler tests
- [ ] Test `ComputeNextRun` for cron (verify next occurrence after reference time)
- [ ] Test `ComputeNextRun` for interval (verify from.Add behavior)
- [ ] Test `ComputeNextRun` for once (verify nil return)
- [ ] Test `PollOnce` with due task — verify execFn called, run recorded, next_run advanced
- [ ] Test `PollOnce` with not-due task — verify execFn NOT called
- [ ] Test `PollOnce` with paused task — verify skipped
- [ ] Test `PollOnce` with once-task — verify paused after execution
- [ ] Test `PollOnce` with failing execFn — verify error recorded, scheduler continues
- [ ] Test overlap prevention — set next_run in past, call PollOnce, verify next_run advanced before exec
- [ ] Test stale run recovery on startup
- [ ] Integration test: wire store + mock execFn, add past-due task, PollOnce, assert full lifecycle
- [ ] Verify: `CGO_ENABLED=0 go test ./internal/scheduler/...` passes
- [ ] Commit: `feat: add scheduler polling loop with sequential execution`

---

## Phase 4: CLI Integration

### Task 11: Add schedule CLI commands to main.go
- [ ] Add `dataDir()` function (XDG_DATA_HOME with fallback, `filepath.Join`)
- [ ] Add `openSchedulerStore()` helper (opens store at `filepath.Join(dataDir(), "tasks.db")`)
- [ ] Add `scheduleCmd()` returning `*cobra.Command` with Use="schedule", Short="Manage scheduled tasks"
- [ ] Add `scheduleAddCmd()`:
  - Flags: `--name` (required), `--cron`, `--interval`, `--once` (mutually exclusive, one required), `--mode` (required, validate "inject"/"isolated"), `--prompt` (required)
  - Validate schedule expression via gronx/ParseDuration/ParseRFC3339
  - Compute initial `next_run_at` via `ComputeNextRun`
  - Call `store.AddTask`, print confirmation with ID and next run time
- [ ] Add `scheduleListCmd()`:
  - Call `store.ListTasks()`
  - Print formatted table (ID, Name, Type, Schedule, Mode, Status, Next Run, Last Run, Last Status)
- [ ] Add `scheduleRemoveCmd()`:
  - Accept positional arg (ID or name)
  - `store.ResolveTask` then `store.RemoveTask`
- [ ] Add `schedulePauseCmd()` and `scheduleResumeCmd()`:
  - Accept positional arg
  - `store.ResolveTask` then `store.SetPaused`
- [ ] Register `scheduleCmd()` in `rootCmd.AddCommand(...)` alongside existing commands

### Task 12: Wire scheduler into _run command
- [ ] In `runCmd()` RunE, after `os.Chdir`:
  - Open store: `store, err := scheduler.NewStore(filepath.Join(dataDir(), "tasks.db"))`
  - If err: log warning, continue without scheduler
  - Create scheduler: `sched := scheduler.New(store, cfg.ClaudeBinary, 60*time.Second)`
  - Create context: `ctx, cancel := context.WithCancel(context.Background())`
  - Start: `go sched.Run(ctx)`
  - Before the restart loop, defer: `cancel(); sched.Wait(); store.Close()`
- [ ] Verify: `go build -o ./hermitclaw . && ./hermitclaw schedule list` works (empty list)
- [ ] Verify: `./hermitclaw schedule add --name test --cron "0 9 * * *" --mode isolated --prompt "hello"` works
- [ ] Verify: `./hermitclaw schedule list` shows the task
- [ ] Verify: `./hermitclaw schedule remove test` works
- [ ] Commit: `feat: add schedule CLI commands and wire scheduler into _run`

---

## Phase 5: Skill and Packaging

### Task 13: Create schedule skill
- [ ] Create `skills/schedule/SKILL.md` with:
  - Frontmatter: name, description
  - All subcommands with examples
  - Mode explanations (inject sends Telegram message agent sees; isolated spawns separate claude)
  - Schedule types (cron, interval, once)
  - Notes: 60s poll granularity, once-tasks pause after execution (resume to retry), tasks persist across restarts

### Task 14: Update flake.nix
- [ ] Run `nix build` to get the new vendorHash (will fail with expected hash)
- [ ] Update `vendorHash` in flake.nix
- [ ] Add `skills.schedule = lib.mkDefault ./skills/schedule;` to skill registration
- [ ] Verify: `nix flake check` passes

### Task 15: Update install.sh
- [ ] Read `install.sh`
- [ ] Add installation of `self-management` skill (missing from current installer)
- [ ] Add installation of `schedule` skill
- [ ] Commit: `feat: add schedule skill, update packaging`

### Task 16: Update CLAUDE.md
- [ ] Read `CLAUDE.md`
- [ ] Add `internal/scheduler/` to architecture diagram
- [ ] Fix stale "TOML parsing: Flat key=value only, grep/sed, never eval'd" claim
- [ ] Add note about scheduler and SQLite storage
- [ ] Commit: `docs: update CLAUDE.md with scheduler architecture`

### Task 17: Final verification
- [ ] `just check` passes
- [ ] `nix flake check` passes
- [ ] Manual smoke test: add a cron task, verify it appears in list, remove it
- [ ] Manual smoke test: add an inject task due in 1 minute, verify Telegram message arrives
