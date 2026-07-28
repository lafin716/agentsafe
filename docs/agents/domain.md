# Domain Docs

How the engineering skills should consume this repository's domain documentation when exploring the codebase.

## Before exploring, read these

- **`CONTEXT.md`** at the repository root.
- **`docs/adr/`** for ADRs affecting the area being changed.

If these files do not exist, proceed silently. Do not flag their absence or suggest creating them upfront. The `/domain-modeling` skill, reached through `/grill-with-docs` and `/improve-codebase-architecture`, creates them lazily when terms or decisions are resolved.

## File structure

This repository uses the single-context layout:

```text
/
├── CONTEXT.md
├── docs/
│   └── adr/
│       ├── 0001-example-decision.md
│       └── 0002-another-decision.md
├── apps/
├── internal/
└── packages/
```

## Use the glossary's vocabulary

When output names a domain concept—such as in an issue title, refactor proposal, hypothesis, or test name—use the term defined in `CONTEXT.md`. Do not drift to synonyms that the glossary explicitly avoids.

If a needed concept is absent from the glossary, reconsider whether the project uses that language or note the gap for `/domain-modeling`.

## Flag ADR conflicts

If output contradicts an existing ADR, surface the conflict explicitly instead of silently overriding the decision.
