---
name: implement-feature
description: Implement one scoped feature or milestone task from the product and technical specifications, with tests and verification.
argument-hint: "<feature or task description>"
---

# Implement Feature Workflow

Task: $ARGUMENTS

1. Inspect git status and current repository structure. Do not overwrite unrelated user changes.
2. Read `AGENTS.md`, `CLAUDE.md`, relevant rules, and relevant sections of all three docs.
3. Identify the smallest deliverable and acceptance criteria. If the request is ambiguous or conflicts with the spec, ask before making a broad assumption.
4. Present a concise implementation plan for non-trivial changes.
5. Implement in small, reviewable steps. Follow existing patterns.
6. Add or update tests for success, failure, isolation, and edge cases.
7. Run formatting, targeted tests, relevant full tests, and race tests for concurrency-sensitive work.
8. Inspect the final diff for accidental scope expansion, secrets, generated noise, and unrelated changes.
9. Report changed files, tests actually run, results, and remaining risks.
