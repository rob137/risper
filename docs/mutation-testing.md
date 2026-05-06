# Mutation Testing

Mutation testing is a real, established testing practice: deliberately change small pieces of production code and check whether the tests fail.

Current Risper state:

- `scripts/mutation-smoke.sh` is a tiny dependency-free smoke check.
- It copies the repo to `/tmp`, breaks selected-model resolution, and expects the tests to fail.
- `scripts/mutmut.sh` runs the real `mutmut` mutation tester through `uvx`.
- `mutmut` is initially scoped to stable core modules rather than GTK/desktop integration code.

Likely proper Python tools:

- `mutmut`
- `cosmic-ray`

Run:

```bash
./scripts/test.sh
./scripts/mutation-smoke.sh
./scripts/mutmut.sh run
./scripts/mutmut.sh results
```

Notes:

- Keep the smoke mutation script as a cheap CI/local check.
- Use `mutmut` for deeper confidence before larger behavior changes.
- Expand `paths_to_mutate` as tests mature around more modules.
