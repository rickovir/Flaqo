# Workflow entry points

The workflows are exposed as Claude Code skills (slash commands):
- `/implement-feature <task>` — scoped implementation with tests.
- `/bug-fix <bug>` — reproduce, diagnose, fix, regression test.
- `/release-review [milestone]` — pre-release quality and security review.

Domain skills can be invoked directly:
- `/feature-flag-domain`
- `/go-backend-development`
- `/postgres-database`
- `/realtime-sse`
- `/authentication-security`
- `/typescript-sdk`
- `/quality-review`

Use subagents for independent read-only reviews; keep implementation in the main session unless parallel work is explicitly planned.
