# Development environment

This document records the toolchain this repository is developed and
verified against. It exists because two of the tools need non-obvious
setup on Windows.

## Toolchain

| Tool | Version | Location |
| ---- | ------- | -------- |
| Go | 1.27.4 | `C:\tools\go` |
| GCC (MinGW-W64) | 16.2.0 | `C:\tools\mingw64` |
| Node.js | 24.x | system |
| golang-migrate | v4.20.1 | `$(go env GOPATH)/bin` |

Both `C:\tools\go\bin` and `C:\tools\mingw64\bin` are on the user PATH.

## Why GCC is required

`go test -race` needs cgo, and cgo needs a C compiler. Without GCC the
race detector cannot run, and the race detector is required by the
testing rules for concurrency-sensitive code.

## Two Windows-specific gotchas

### 1. The toolchain must live in a path without spaces

The account path contains a space ("Ricko Virnanda"). MinGW's linker
splits arguments on spaces and fails with:

``` text
ld.exe: cannot find C:/Users/Ricko: No such file or directory
```

Installing GCC under the account path does not work. It must be copied
to a space-free path such as `C:\tools\mingw64`.

The same class of problem affects any tool that shells out to a compiler.
If a build fails with a path error mentioning a truncated path, move the
toolchain rather than changing quoting.

### 2. `golang-migrate` needs the postgres driver built in

The stock `go install` of the CLI produces a binary that fails with
`database driver: unknown driver postgresql`. The postgres driver is
behind a build tag:

``` bash
go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@v4.20.1
```

## make

The root `Makefile` expects `make` on PATH. On Windows, install it with:

``` bash
winget install --id GnuWin32.Make --exact
```

The individual commands work without make and are what the Makefile
wraps:

``` bash
cd apps/api
gofmt -w $(find . -name '*.go' -not -path './sql/generated/*')
go vet ./...
go test ./...
go test -race ./...
```

## Migrations

Migrations run as the `ff_migrator` role, never as the runtime role:

``` bash
make migrate-up        # apply
make migrate-down      # roll back one
make migrate-version   # show current version
```

The `MIGRATION_DATABASE_URL` must point at the migration role. The
Supabase MCP server is read-only and cannot apply migrations.

## Environment

Copy `.env.example` to `.env` and fill in the values. `.env` is
gitignored. The two connection strings point at different roles:

- `DATABASE_URL` uses `ff_runtime`, which has DML only.
- `MIGRATION_DATABASE_URL` uses `ff_migrator`, which owns the schema.

`CSRF_SECRET` must be at least 32 bytes. The server refuses to start
without it.

Generate secrets with:

``` bash
openssl rand -base64 32
```
