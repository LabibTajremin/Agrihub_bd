# BUILD STATE
Updated: 2026-09-28
Branch: claude/festive-maxwell-n5xdmi
PR: #1 (draft)
Last commit: phase 16
CI: 12 green; 14 red (SystemClock const ctor coverage) fixed in 6919683; 15–16 pending

## PHASES
| # | Name | Status | Commit |
|---|------|--------|--------|
| 0 | Bootstrap & guardrails | done | 6b0ac80 |
| 1 | Platform kernel | done | 04ec047 |
| 2 | Persistence & migrations | done | cb5e536 |
| 3 | Identity (auth + RBAC) | done | cbc3b7f |
| 4 | Localization (dictionary + voice) | done | 901e9e0 |
| 5 | Farm | done | 7f1c504 |
| 6 | Media | done | d1a0b75 |
| 7 | Diagnosis | done | 8704d16 |
| 8 | Advisory | done | 361c350 |
| 9 | Weather & Alert | done | ac99c43 |
| 10 | AI adapter (OPEN SLOT) | done | de74f01 |
| 11 | API assembly & deploy targets | done | 6ce7757 |
| 12 | Flutter foundation | done | d222468 |
| 13 | Flutter onboarding & auth | done | 9c3f2f0 |
| 14 | Flutter home & dashboard | done | 4056a79 (+34bcd73 lint fix) |
| 15 | Flutter plant doctor | done | 3c15807 |
| 16 | Flutter crop advisor + chart narration | done | (this) |
| 17 | Flutter voice shell + settings | todo | — |
| 18 | Hardening & handoff | todo | — |

## NEXT ACTION
Phase 17: mobile features/voice (idle/listening/understanding/response/not-understood/help → POST /v1/assistant/ask stub; recorder behind a port, no STT) + settings (language, offline models, expert help, profile, privacy, sign out).

## BLOCKERS
none (after a container restart run `dockerd &` for integration tests)

## DECISIONS THIS PHASE
- BarChart always narrated; clip delivery client-side by checksum; AudioOut/LocationGateway ports
