---
name: release-review
description: Perform a pre-release review of the current diff or milestone against MVP acceptance criteria, security, testing, and operational readiness.
argument-hint: "[milestone or release candidate]"
---

# Release Review Workflow

Target: $ARGUMENTS

1. Read the MVP acceptance criteria and release checklist.
2. Inspect git diff, repository status, migrations, and test scripts.
3. Verify project/environment isolation and SDK credential boundaries.
4. Verify database transaction/version ordering and SSE reconnect behavior.
5. Verify session security, CSRF, rate limiting, and secret handling.
6. Run available tests and checks; clearly identify anything not runnable.
7. Review migration safety and deployment configuration.
8. Produce a prioritized findings report: Critical, High, Medium, Low.
9. Do not deploy, run destructive migrations, or modify production data.
10. Give a release recommendation: Ready, Ready with follow-ups, or Blocked.
