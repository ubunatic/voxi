# Ideas for `internal/doctor` improvements

1. **Extract shared polling loops into a helper function**
   - **Change:** Refactor the polling loops in `waitForGraph` and `awaitStart` to use a unified `poll` helper in `internal/doctor/host.go` that takes a condition function and evaluates it repeatedly with `d.Host.Wait` up to a given timeout.
   - **Why it helps:** Both functions implement ad-hoc `time.Duration` accounting and error checking with `d.Host.Wait`. Centralizing this makes the polling logic DRY, easier to read, and simpler to write fake-clock unit tests for.
   - **Citation:** `internal/doctor/fix.go:203` (and `internal/doctor/fix.go:434`)
   - **Smallest step:** Add a `poll(ctx context.Context, host Host, timeout, interval time.Duration, condition func() (bool, error)) error` function to `internal/doctor/host.go`.

2. **Add a mockable clock to `Host` for precise time-based testing**
   - **Change:** Extend the mock `Host` implementations and product code to consistently use `d.Host.Now()` for generating timestamps (such as in `snapshot`), instead of calling `time.Now()` directly.
   - **Why it helps:** Currently, time-dependent tests (like `TestSnapshotPrunesOldFiles`) must create snapshot files with timestamps in the past relative to the real `time.Now()`. Relying strictly on the `Host` clock ensures complete test determinism and isolation from the real clock.
   - **Citation:** `internal/doctor/fix.go:379`
   - **Smallest step:** Update `HostDoctor.snapshot` to call `d.Host.Now()` instead of `time.Now()` and pass that timestamp down to `pruneSnapshots`.

3. **Standardize command execution error handling in tests**
   - **Change:** Update the `fixtureHost` mock in `internal/doctor/doctor_test.go` to return structured execution errors (like `exec.ErrNotFound` or `*exec.Error`) instead of strings wrapped in `fmt.Errorf`.
   - **Why it helps:** The `commandSkip` helper explicitly checks for `*exec.Error` and `exec.ErrNotFound` to determine if a tool is missing entirely. The current mock `healthyHost` returns generic `fmt.Errorf`, which prevents tests from properly exercising these "missing tool" code paths.
   - **Citation:** `internal/doctor/doctor.go:633`
   - **Smallest step:** Modify `healthyHost` in `internal/doctor/doctor_test.go` to return an `&exec.Error{Name: name, Err: exec.ErrNotFound}` when simulating an missing command.

4. **Improve JSON decoding efficiency in graph parsing**
   - **Change:** In `parseGraph`, replace `json.Unmarshal` reading the entire byte array into memory as a slice of maps, with a streaming `json.Decoder` that processes objects iteratively.
   - **Why it helps:** `pw-dump` JSON output can be quite large. Iterative decoding reduces memory allocations and peak RAM usage during the doctor check, helping `voxi doctor` remain lightweight on constrained hardware.
   - **Citation:** `internal/doctor/doctor.go:550`
   - **Smallest step:** Replace the `json.Unmarshal(data, &objects)` call in `parseGraph` with a `json.NewDecoder(bytes.NewReader(data))` loop that reads tokens sequentially.

5. **Separate core dump parsing from `checkCoredumps` logic**
   - **Change:** Extract the logic that parses the lines from `coredumpctl` into a dedicated helper function, separated from the command execution and result generation.
   - **Why it helps:** `checkCoredumps` currently mixes command execution, parsing the text output, and result formatting. Extracting the text parsing logic makes it independently testable without needing a full `Host` mock.
   - **Citation:** `internal/doctor/doctor.go:394`
   - **Smallest step:** Create `func parseLatestCoredump(text string, processes []string) (hits []string)` in `internal/doctor/doctor.go` and call it from `checkCoredumps`.
