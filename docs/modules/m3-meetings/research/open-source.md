# M3 Meetings - research: open-source alternatives and building blocks

**Accessed:** 2026-11-02. Stars, licence (SPDX from the GitHub API, LICENSE file where the API says NOASSERTION), last push
and latest release were read from `api.github.com` on that date. "Fit" values: **embed** = Go library linked into the
JAAS backend; **integrate** = run as a separate service behind a port; **learn** = read only, no code reuse.
JAAS rule (M2 DPJ-01): no copied code; libraries only with a permissive licence (MIT/BSD/Apache). AGPL/GPL components can
only be integrated as separate, unmodified services, and only after an owner decision.

## 1. Scheduling, calendars, CalDAV

| Project | Licence | Maturity (2026-11-02) | How it works | Fit |
|---|---|---|---|---|
| **Cal.diy** (was `calcom/cal.com`) | MIT (was AGPL-3.0 until Apr 2026) | ~49.6k stars, active; last tagged release v6.2.0 (2026-03-01); public repo is a reduced community fork | Next.js + Prisma + Postgres. Each booking is a row with `uid`, `startTime/endTime`, `status`, `recurringEventId`, `iCalUID/iCalSequence`, unique `idempotencyKey`. External calendars are polled/watched into `CalendarCacheEvent`; busy times are merged and checked in memory. | **learn** (TypeScript, scope reduced, closed upstream) |
| **sabre/dav** | BSD-3-Clause | 1.7k stars, release 4.7.1 (2026-07-07); powers Nextcloud and Baikal | PHP CalDAV/CardDAV server. Stores the raw iCalendar blob plus denormalised `firstoccurence/lastoccurence`, `uid`, `etag`; changes are journalled in `calendarchanges` with a per-calendar `synctoken` (RFC 6578). | **learn** (best reference for CalDAV storage) |
| **Baikal** | GPL-3.0 | 3.4k stars, pushed 2026-10-19 | Thin admin UI + sabre/dav server with MySQL/SQLite. | learn |
| **Radicale** | GPL-3.0 | 5.2k stars, release v3.9.0 (2026-10-25) | Python CalDAV/CardDAV server storing one `.ics` file per item in a filesystem collection; simple, small deployments. | learn (or integrate as a test peer for CalDAV client tests) |
| **Nextcloud Calendar / server** | AGPL-3.0 | calendar 1.2k, server 37.4k stars, active | Vue calendar app on top of Nextcloud's sabre-based CalDAV backend; room/resource booking backends, free/busy. | learn |
| **emersion/go-webdav** | MIT | 501 stars, v0.7.0 (2025-10-18), pushed 2026-06 | Go WebDAV/CalDAV/CardDAV client and server. Server = `caldav.Backend` interface you implement (List/Get/Query/Put/Delete calendar objects). Client supports `sync-collection`. | **embed** for a later read-only CalDAV endpoint or CalDAV client sync |
| **emersion/go-ical** | MIT | 73 stars, last push 2025-06 | Go iCalendar encoder/decoder; `Component.RecurrenceSet()` builds an rrule-go set from RRULE/RDATE/EXDATE. | embed (with go-webdav) |
| **arran4/golang-ical** | Apache-2.0 | 422 stars, v0.3.8 (2026-10-18), fuzz tests | Go iCalendar parser/serialiser, RRULE parse/format (no expansion), Windows <-> IANA zone mapping (Outlook). Has `ParseCalendarFromUrl` (must not be used: SSRF). | **embed** for `.ics` export/feed |
| **teambition/rrule-go** | MIT | 384 stars, no releases, last push 2024-08 (stable port of python-dateutil) | Iterator-based RFC 5545 RRULE engine + `Set` (RRULE, RDATE, EXDATE). `Between()` iterates from DTSTART. | **embed** for expansion (wrap with our own bounds) |

## 2. Video

| Project | Licence | Maturity | How it works | Fit |
|---|---|---|---|---|
| **Jitsi Meet** | Apache-2.0 | 30.2k stars, stable release 2026-09-14 | Web app + Prosody XMPP + Jicofo + Jitsi Videobridge (SFU). Rooms by name; with the token plugin a signed JWT (shared secret or public key) authenticates the user and can make them moderator. | **integrate** (self-hosted; M3 only mints JWT join links) |
| **LiveKit** | Apache-2.0 | 22.0k stars, v1.13.11 (2026-10-25) | Go SFU with SDKs; clients join with a backend-minted JWT access token; egress for recording, agents framework for AI. | **integrate** (best fit for embedded video later; Go server SDK) |
| **BigBlueButton** | LGPL-3.0 | 9.3k stars, v4.0.1 (2026-10-29) | Classroom-oriented conferencing (whiteboard, breakout rooms, recordings) with a create/join API signed by a shared secret checksum. | integrate only for training/classroom use |
| **Nextcloud Talk (spreed)** | AGPL-3.0 | 2.2k stars, active | Chat + calls inside Nextcloud, optional HPB signalling server. | learn |

## 3. Collaborative notes

| Project | Licence | Maturity | How it works | Fit |
|---|---|---|---|---|
| **Yjs** | MIT | 23.0k stars, active | CRDT for shared types; documents are binary updates merged commutatively; offline-first. | integrate in the client if MN-5 real-time editing is chosen |
| **y-crdt (Yrs)** | MIT | 2.2k stars, active | Rust port of Yjs with C/Wasm/Python bindings; no official Go binding. | learn (Go server would treat updates as opaque bytes) |
| **Hocuspocus** | MIT | 2.6k stars, v4.7.1 (2026-10-12) | Node.js Yjs WebSocket server with auth hooks and persistence hooks (store/load document). | integrate (separate Node service) only if CRDT path chosen |
| **Tiptap** | MIT (core) | 38.9k stars, active | ProseMirror-based editor; Collaboration extension uses Yjs; markdown import/export. | client editor (embed in frontend) |
| **Etherpad** | Apache-2.0 | 18.6k stars, active | Node pad server using operational transformation, full revision history per pad. | learn (revision model) |
| **HedgeDoc** | AGPL-3.0 | 7.5k stars, release 1.12.0 (2026-08-21) | Collaborative markdown editor (v2 rewrite uses Yjs). | learn |

## 4. Transcription and meeting-minutes pipelines (for M9, not M3)

| Project | Licence | Maturity | How it works | Fit |
|---|---|---|---|---|
| **whisper.cpp** | MIT | 54.8k stars, v1.9.5 (2026-10-06) | C/C++ Whisper inference on CPU/GPU (Metal, CUDA, Vulkan); server example with HTTP API. | M9 integrate (local STT worker) |
| **faster-whisper** | MIT | 26.4k stars, v1.2.1 (2025-10-31) | Whisper on CTranslate2 (int8/fp16), faster and lower memory than reference. | M9 integrate (Python worker) |
| **WhisperX** | BSD-2-Clause | 25.0k stars, pushed 2026-10-22 | faster-whisper + forced alignment (word timestamps) + pyannote diarization (speaker labels; pyannote models need their own terms). | M9 integrate when speaker labels needed |
| **openai/whisper** | MIT | 111.7k stars | Reference PyTorch implementation. | learn |
| **Meetily** (`Zackriya-Solutions/meetily`) | MIT | 32.4k stars, active | Tauri (Rust) desktop app: local audio capture, Whisper/Parakeet transcription, summaries via Ollama or cloud LLMs; paid PRO edition exists. | learn (bot-free local capture design) |
| **Anarlog** (was Hyprnote, then Char; `fastrepl/anarlog`) | MIT (relicensed from GPL) | 9.7k stars, active | Local-first desktop notetaker that enhances the user's notes with transcripts; renamed twice in 2026. | learn |
| **Vexa** | Apache-2.0 | 2.9k stars, v0.12.28 (2026-10-09) | Self-hostable bots join Meet/Teams/Zoom (Jitsi pending), stream speaker-attributed transcripts via faster-whisper; gateway + Postgres + Redis; MCP endpoint. | M9 integrate candidate (bot-based capture), isolated service |

## 5. Shortlist for JAAS

1. **Embed (Go, permissive):** `teambition/rrule-go` (expansion), `arran4/golang-ical` (iCal export/feed), later
   `emersion/go-webdav` (CalDAV). All three were read in `internals.md`.
2. **Integrate as separate services, later phases:** LiveKit or Jitsi (video links with signed tokens); Hocuspocus+Yjs only
   if real-time co-editing is approved; whisper.cpp/faster-whisper (and possibly Vexa) owned by M9.
3. **Learn only:** sabre/dav storage + sync tokens, Cal.diy booking model and idempotency keys, Etherpad revisions.

## 6. Sources (accessed 2026-11-02)

- GitHub API `https://api.github.com/repos/<owner>/<repo>` and `/releases/latest` for every repo above (stars, licence, push date, release).
- Repos: https://github.com/calcom/cal.diy , https://github.com/sabre-io/dav , https://github.com/sabre-io/Baikal , https://github.com/Kozea/Radicale , https://github.com/nextcloud/calendar , https://github.com/emersion/go-webdav , https://github.com/emersion/go-ical , https://github.com/arran4/golang-ical , https://github.com/teambition/rrule-go , https://github.com/jitsi/jitsi-meet , https://github.com/livekit/livekit , https://github.com/bigbluebutton/bigbluebutton , https://github.com/nextcloud/spreed , https://github.com/yjs/yjs , https://github.com/y-crdt/y-crdt , https://github.com/ueberdosis/hocuspocus , https://github.com/ueberdosis/tiptap , https://github.com/ether/etherpad , https://github.com/hedgedoc/hedgedoc , https://github.com/ggml-org/whisper.cpp , https://github.com/SYSTRAN/faster-whisper , https://github.com/m-bain/whisperX , https://github.com/openai/whisper , https://github.com/Zackriya-Solutions/meetily , https://github.com/fastrepl/anarlog , https://github.com/Vexa-ai/vexa
- Cal.diy relicensing: https://cal.com/fr/blog/cal-diy-open-source-to-closed-source ; https://pinggy.io/blog/self_hosted_calendly_alternatives/
- Anarlog rename: https://anarlog.so/blog/char-is-now-anarlog ; https://char.com/blog/hyprnote-is-now-char
- Meetily and Vexa READMEs (fetched 2026-11-02).
- Jitsi JWT: https://linpro.no/techblog/2023/03/08/Jitsi-with-JWT-and-Moderated-Meetings.html ; LiveKit tokens: https://docs.livekit.io/home/get-started/authentication/ (grant field details on the linked tokens-grants page were not fetched).
