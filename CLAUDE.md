# AgriSmart — repo guide for AI sessions

## Standing instructions
The user's reusable build preferences live in **`docs/agent/STANDING_INSTRUCTIONS.md`**.
When the user asks for "my generic instructions", return that file's contents.

## Build spec
The full autonomous build plan is **`docs/agent/BUILD_INSTRUCTION.md`** (phases 0–18).
Live progress is tracked in **`.agent/STATE.md`** — read it first in any build session.

## Quick facts
- Backend: Go, API-only, clean architecture, repository pattern, modular monolith (microservice-ready)
- Frontend: Flutter, mobile-first
- Branch: `claude/sleepy-mccarthy-h8hv7s` · one commit per phase · one PR, merged by the user
- Coverage gate: 100% unit + integration, backend and mobile
- The AI/ML model is an **open slot** — interfaces and a stub only, never implemented
- Design reference: Figma `AgriSmart — Farmer App (User App) UI Kit & Flows` (58 screens, 25 components)
