module github.com/blairham/pre-commit-hooks

// Deliberately low, with no dependencies: pre-commit's golang backend installs
// hooks with GOTOOLCHAIN=local, so this module must build on whatever Go a
// consumer happens to have on PATH. Raise it only when something here needs it.
go 1.22.0
