# Claude Code AI Toolkit

This toolkit configures Claude Code for the Feature Flag Platform repository.

## Included

- `CLAUDE.md`: Claude Code entry-point instructions.
- `AGENTS.md`: portable repository-wide instructions for other coding agents.
- `.claude/rules/`: path-scoped engineering and security constraints.
- `.claude/skills/`: domain skills and slash-command workflows.
- `.claude/agents/`: read-only backend, security, and test reviewers.
- `.mcp.json`: project MCP configuration.
- `.claude/settings.json`: conservative deny rules.
- `.claude/settings.local.json.example`: optional local command permissions.
- `docs/`: the product, technical, and implementation blueprints.

## Prerequisites

- Claude Code installed and authenticated.
- Node.js 20+ for the MCP servers shown here.
- Go toolchain matching the version pinned in `apps/api/go.mod` once created.
- A dedicated Supabase development project.

## Setup

1. Copy the toolkit contents into the root of `feature-flag-platform`.
2. Ensure the three blueprint files are in `docs/`.
3. Set `SUPABASE_PROJECT_REF` in your shell to the ref of your dedicated development project. `.mcp.json` reads it as `${SUPABASE_PROJECT_REF}`, so no edit to the tracked file is needed. Keep `read_only=true`.
4. Set `CONTEXT7_API_KEY` in your shell if you have one (optional), then start Claude Code from the repository root.
5. Approve the project MCP servers when prompted. Complete Supabase OAuth if requested.
6. Run `/mcp` and verify Supabase, Context7, and Playwright connectivity.
7. Run `/skills` (or inspect available skills) and verify the project skills are discovered.
8. Copy `.claude/settings.local.json.example` to `.claude/settings.local.json` and adjust permissions to your environment.

### Secrets handling

Tracked configuration files must never contain live credentials.

- `.claude/settings.json` holds only non-secret settings and permission rules. Put `ANTHROPIC_AUTH_TOKEN` and any other credential in `.claude/settings.local.json` (gitignored) or in your shell environment.
- `.mcp.json` refers to credentials by environment variable (`${CONTEXT7_API_KEY}`), never by literal value.
- Application configuration goes in `.env` (gitignored). Commit `.env.example` with placeholders only.
- The Supabase MCP server is intentionally `read_only=true`. It cannot create schemas or apply migrations. Schema changes go through `golang-migrate` over a direct connection using `MIGRATION_DATABASE_URL`; the MCP is for reading schema state and documentation.

If a credential is ever committed, rotate it. Removing the value from the file does not invalidate a leaked token.

Example shell setup (macOS/Linux):
```bash
export CONTEXT7_API_KEY="your-context7-api-key" # optional
claude
```

Windows PowerShell:
```powershell
$env:CONTEXT7_API_KEY = "your-context7-api-key"
claude
```

The Supabase MCP URL is project-scoped and read-only. This is intentional for normal coding. Apply migrations through the reviewed migration workflow and normal database credentials, not by granting the agent broad database write access by default.

## MCP notes

- Supabase: hosted HTTP MCP, scoped to one development project, `read_only=true`, database/docs feature groups only. Supabase OAuth may be required by Claude Code.
- Context7: local stdio MCP via `npx`. An API key is optional depending on usage/rate limits.
- Playwright: local stdio MCP. Browser downloads may occur on first use.
- If an MCP server is not needed for a task, it can be disabled in Claude Code's MCP controls.
- Do not add production project refs or production credentials to this repository.
- Review upstream package versions before pinning them for a production team setup.

## First-run validation

From the repository root:
```bash
claude
```
Then in Claude Code:
```text
/mcp
/feature-flag-domain
```
Ask Claude to summarize the MVP invariants and identify any contradictions among the three docs. It should not modify files during this validation.

## Recommended development loop

1. `/implement-feature <small scoped task>`
2. Review the plan before allowing broad changes.
3. Implement and test.
4. Ask `backend-reviewer`, `security-reviewer`, or `test-reviewer` for independent read-only review.
5. `/quality-review`
6. Inspect `git diff` yourself before commit.

## Safety

- Keep MCP read-only for Supabase during normal development.
- Never give an agent production database credentials.
- Never approve destructive SQL, resets, drops, production migrations, deployments, or secret rotation without personally reviewing the exact action.
- `.env*`, local settings, and credentials must remain untracked.
