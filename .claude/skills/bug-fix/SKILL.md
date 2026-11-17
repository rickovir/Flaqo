---
name: bug-fix
description: Diagnose and fix a reproducible defect while minimizing scope and adding a regression test.
argument-hint: "<bug description>"
---

# Bug Fix Workflow

Bug: $ARGUMENTS

1. Inspect git status and preserve unrelated changes.
2. Reproduce the issue or identify the failing test/log evidence. Do not guess at root cause.
3. Trace the behavior through the relevant module boundaries.
4. Explain the confirmed root cause and a minimal fix plan.
5. Add a regression test that fails before the fix when practical.
6. Implement the smallest safe change.
7. Run targeted tests and broader relevant checks.
8. Review for regressions in project/environment isolation, configuration versions, SSE, and SDK behavior where relevant.
9. Report evidence, changed files, test outcomes, and any remaining uncertainty.
