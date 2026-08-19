# Mutation Testing

Mutation testing deliberately changes a small piece of production code and checks that the tests fail.

Risper keeps one focused, dependency-light mutation check:

```bash
./scripts/mutation-smoke.sh
```

The script copies the repository to a temporary directory, runs the complete Go test suite, changes selected-model resolution in the copy, and requires the tests to turn red. The source checkout and user data are not modified.

There is no general mutation runner in the project. The focused smoke catches the model-selection regression that matters to this command surface without adding another toolchain to a small local utility.
