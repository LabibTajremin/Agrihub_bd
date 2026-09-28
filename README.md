# AgriSmart

Offline-first, voice-navigable crop assistant for smallholder farmers in Bangladesh: a leaf-photo
**Plant Doctor** that works without internet, a **Crop Advisor** (suitability, yield, profit,
rotation) with narrated charts, weather alerts, and a voice assistant — in 7 languages including
Bangla (default) and Arabic (RTL).

| | |
|---|---|
| `backend/` | Go API — modular monolith, clean architecture, transactional outbox; Docker and Vercel from one handler tree |
| `mobile/` | Flutter app — offline-first (SQLite queue + cache), runtime language switching |
| `docs/` | developer documentation — start with [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) |

## Quickstart
```bash
make verify                                              # the full gate CI runs (100% coverage, both apps)
docker compose -f deploy/docker/docker-compose.yml up --build   # API on http://localhost:8080
cd mobile && flutter run --dart-define=API_BASE_URL=http://10.0.2.2:8080
```

## AI is an open slot
The disease model, speech recognition, text-to-speech and the conversational agent are **not**
implemented. The system talks to them only through ports (`DiagnosisEngine`, `AdvisoryNarrator`,
`ConversationalAgent` on the server; `DiagnosisEngine`, `ModelSource`, `VoiceInput` on the phone),
each with a deterministic stub. See [`docs/AI_INTEGRATION.md`](docs/AI_INTEGRATION.md).

## Docs
[Architecture](docs/ARCHITECTURE.md) · [API](docs/API.md) · [Testing](docs/TESTING.md) ·
[Localization](docs/LOCALIZATION.md) · [Deployment](docs/DEPLOYMENT.md) · [Decisions](docs/DECISIONS.md)
