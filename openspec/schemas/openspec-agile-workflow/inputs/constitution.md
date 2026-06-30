# Constitution Template

This file is a **placeholder**. Replace it with your operator's constitution document.

The constitution defines non-negotiable guardrails, coding conventions, development workflow,
and governance rules derived from your operator's codebase. It is used as an input for the
Planning, Task Creation, and Implementation stages.

## How to provide constitution.md

The workflow resolves `constitution.md` using this lookup order:

1. `{target_repo}/constitution.md` — checked first (if target repo has one)
2. `{target_repo}/CONSTITUTION.md`
3. `{schema_root}/inputs/constitution.md` — this file (schema inputs/ fallback)

## What to include

Your constitution should contain:

- **Core Principles** — repo-evidence-backed conventions (controller patterns, test-first, generated code discipline, RBAC)
- **Additional Constraints** — tech stack, compliance, naming conventions
- **Development Workflow** — review process, CI gates, local verify commands, codegen refresh
- **Agent Routing** — AgentRoutingMode (PROVIDED/PROVISIONAL), agent ID table
- **Governance** — how the constitution relates to AGENTS.md, CONTRIBUTING.md, precedence rules

See `templates/constitution-template.md` for the full output template structure you can follow.

## Generating a constitution

You can generate a constitution for your operator by:

1. Running the workflow with the constitution template (`templates/constitution-template.md`)
2. Reviewing and approving the output
3. Saving the approved constitution here for reuse across future changes

Once you have an approved constitution, place it here so subsequent workflow runs
use it as a pre-approved input rather than regenerating it each time.
