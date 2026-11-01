# M3 Meetings - research: market products

**Accessed:** 2026-11-02. **Pricing caveat:** vendor pricing pages were not fetched directly; figures come from third-party
trackers/reviews that often disagree. Treat every price as "approximate, verify on the vendor page". Where a fact could not
be verified it says so.

## 1. Product table

| Product | Key features | Pricing model (one line) | Copy | Avoid |
|---|---|---|---|---|
| **Google Calendar + Meet** | Calendar with RRULE recurrence, rooms as resources, free/busy, Meet links auto-added; Gemini "Take notes for me" writes a Doc with Summary / Decisions / Next steps (sections selectable since Apr 2026); in-person note taking announced Aug 2026 | Per user Workspace plans; note-taking tied to organizer's edition (sources conflict which editions) | Notes as a *document attached to the event*; fixed sections "Summary, Decisions, Next steps"; incremental sync API (`syncToken`, 410 = full resync) | Feature availability depending on organizer's licence: confusing for attendees |
| **Microsoft Outlook + Teams** | Exchange calendar, rooms/room lists, Teams meeting per event, Loop/notes, "intelligent recap" (Teams Premium or Microsoft 365 Copilot), transcripts | Teams Premium about $10/user/mo add-on; Copilot about $30/user/mo (third-party, verify) | Recap = notes + tasks + chapters; Graph `calendarView/delta` and change subscriptions | Licence maze (Premium vs Copilot overlap) |
| **Zoom Workplace** | Meetings, scheduler, AI summaries and "catch me up" included in paid plans; agentic add-on (reported "ZoomMate", about $20/user/mo) | Per host/user tiers; AI included in paid tiers (sources conflict on price) | AI summary included, not an extra SKU | Bot-centric AI that misattributes action items (reported) |
| **Fellow** | Collaborative agendas, templates, action items synced to task tools, AI notes via bot or desktop app, 1:1 and team meeting streams | Free tier + about $7-15/user/mo annual (third-party, verify) | Agenda-first workflow; recurring "stream" with carry-over of open items; meeting guidelines | Very low free AI limits as lock-in |
| **Hypercontext** (Lattice ownership **not verified**) | Shared 1:1/team agendas both sides edit, suggested questions, goals | Basic agendas free (vendor help centre) | 1:1 template with private + shared talking points | - |
| **Lattice** | Performance platform with 1:1 meetings tied to goals/reviews | Per employee per month (not verified) | Link 1:1 notes to HR goals (M1 Performance context) | Meeting notes leaking into performance reviews without consent |
| **Notion AI meeting notes** | AI meeting notes block in a page (desktop app), transcription in ~15-16 languages, notes live with the wiki | Business plan about $20/user/mo annual (third-party) | Notes are ordinary pages, linkable and searchable | Desktop-only capture; AI only on top tier |
| **Confluence meeting notes** | Meeting-notes template, task lists with @mentions, Jira links | Per user Atlassian tiers (not verified this session) | Template: attendees / goals / discussion / action items | Action items as checkboxes in a page, not real tasks - two truths |
| **Otter.ai** | Live transcript, OtterPilot bot joins Zoom/Meet/Teams from calendar, summaries, action items | Free 300 min/mo; Pro/Business per seat (third-party, conflicting) | Calendar-driven auto-join; speaker-labelled transcript | Bots auto-joining meetings without clear consent |
| **Fireflies.ai** | Bot recorder, transcripts, AI summaries, CRM integrations | Free + Pro about $10, Business about $19, Enterprise about $39/user/mo annual (third-party) | Searchable transcript library with topic trackers | AI-credit metering that makes cost unpredictable |
| **Read.ai** | Post-meeting reports (summary, chapters, action items, questions), live talk-time/engagement metrics | Free (5/month) + Pro/Enterprise per seat; top tier raised to $39.75 in Apr 2026 (third-party) | Automatic report delivered after each meeting | Engagement/sentiment scoring of people (HR-sensitive, avoid in JAAS) |
| **Granola** | Bot-free: captures device audio, enhances the user's own rough notes with AI | Basic free (30-day history), Business $14, Enterprise $35/user/mo (third-party, consistent) | Human notes first, AI enhances; no bot in the call | Capturing others' audio locally without a consent step |
| **Calendly** | Booking links, routing, round-robin, availability rules, reminders | Standard about $10-12, Teams about $16-20/seat/mo; AI tiers added Aug 2026 (third-party) | Availability rules + buffers; idempotent booking per slot | External-facing booking is a different product - LATER for JAAS |
| **Cal.com / Cal.diy** | Open scheduling; Apr 2026 public repo became **Cal.diy** (MIT, feature-reduced), commercial Cal.com closed source | Commercial per seat; Cal.diy free MIT | Data model ideas (see `internal.md`) | Depending on Cal.diy as a component (reduced scope, maintenance unclear) |
| **Clockwise** | AI calendar defragmentation, focus time | **Shut down 2026-03-27** (team hired by Salesforce; aggregator source) | Focus-time protection idea | Building on a vendor that can vanish |
| **Reclaim.ai** (Dropbox, terms undisclosed) | Habits, tasks, smart 1:1 scheduling, buffers on Google/Outlook | Free Lite + paid per user (sources conflict) | Auto-rescheduling flexible meetings around priorities | - |
| **Asana / monday.com meeting templates** | Meeting agenda/notes templates as projects/boards; action items are native tasks | Part of per-seat work-management plans (not verified this session) | Action item = real task in the work tool (exactly M2 + M3) | Meetings as a fake project: no time zone, recurrence or attendance |
| **Zoho Meeting** | Meetings + webinars, registration, polls, Q&A, recordings, Zoho CRM/Calendar integration | About $1-3/host/mo annual, free tier (third-party, conflicting) | Very low price per host; tight suite integration (like JAAS) | - |

## 2. What the market says for JAAS

1. **The differentiator for JAAS is integration, not AI.** Every standalone tool fights to push action items into a task
   tool and to know who is in which team. JAAS owns both (M2 tasks, M1 org chart): create real tasks from minutes, invite by
   department, attendance reports for HR.
2. **Agenda-first + carry-over** (Fellow, Hypercontext) is the most copied pattern for recurring meetings.
3. **Structured outputs** - Google's notes now have fixed "Summary / Decisions / Next steps" sections; Read.ai and Teams
   recap produce the same triad. M3 should store decisions and action items as rows, not just text, so AI (M9) and
   humans fill the same structure.
4. **AI notetaking is commoditised** (included in Zoom paid plans, Google Workspace, Teams/Copilot; many $10-20 add-ons).
   JAAS should not build transcription in M3; provide seams so M9 can plug in a local model (see `open-source.md`).
5. **Consent and bots** are the main complaint area (auto-joining bots, recording others). Bot-free capture (Granola) and an
   explicit consent flag per meeting are safer defaults.
6. **Vendor risk is real:** Clockwise shut down in one week; Cal.com closed its main repo. Prefer standards (iCalendar,
   CalDAV) and owned code over embedding a vendor.

## 3. Sources (accessed 2026-11-02)

- Google "Take notes for me": https://blog.google/products-and-platforms/products/workspace/take-notes-for-me/ ; in-person notes: https://workspaceupdates.googleblog.com/2026/08/take-notes-with-me-for-in-person-meetings-is-now-available.html ; plan eligibility discussion: https://tactiq.io/learn/how-to-use-google-gemini-note-taker
- Teams Premium vs Copilot: https://www.techtarget.com/searchUnifiedCommunications/tip/Comparing-Microsoft-Teams-Premium-vs-Copilot-in-Teams ; https://www.knowledgewave.com/blog/understanding-microsoft-teams-premium-and-copilot-for-microsoft-365-a-comprehensive-guide
- Zoom AI: https://www.laxis.com/blog/zoom-ai-companion/ ; https://www.nojitter.com/ai-automation/zoom-ai-companion-3-0-now-generally-available
- Fellow: https://meetgeek.ai/blog/fellow-ai-review ; https://getpulsesignal.com/pricing/fellow
- Hypercontext: https://hypercontext.com/features/one-on-ones ; https://help.hypercontext.com/en/articles/1962679-hypercontext-packages
- Notion AI meeting notes: https://www.sally.io/blog/notion-ai-meeting-notes-review ; https://techcrunch.com/?p=3006349
- Otter: https://sonix.ai/resources/otter-ai-review-pricing/ ; https://costbench.com/software/ai-meeting-assistants/otter-ai/free-plan/
- Fireflies: https://sonix.ai/resources/fireflies-ai-pricing/ ; https://www.spinach.ai/blog/fireflies-pricing
- Read.ai: https://pipeline.zoominfo.com/sales/read-ai-features ; https://costbench.com/changelog/read-ai-price-increase-2026-04/
- Granola: https://pricingsaas.com/companies/granola ; https://www.meetjamie.ai/en/blog/granola-ai-pricing
- Calendly: https://cal.com/blog/calendly-pricing ; https://www.usagepricing.com/blueprint/calendly
- Cal.com -> Cal.diy: https://cal.com/fr/blog/cal-diy-open-source-to-closed-source ; https://pinggy.io/blog/self_hosted_calendly_alternatives/ ; GitHub API shows `calcom/cal.com` redirects to `calcom/cal.diy` (checked 2026-11-02)
- Clockwise shutdown: https://usagepricing.com/blueprint/activity/2026-03-27-clockwise-deprecation (aggregator; CostBench shows a conflicting May 2026 snapshot)
- Reclaim/Dropbox: https://techcrunch.com/?p=2841072 ; https://pricingsaas.com/companies/reclaim
- Zoho Meeting: https://costbench.com/software/video-conferencing/zoho-meeting/ ; https://www.capterra.com/p/202121/Zoho-Meeting/pricing/
- Not verified this session: Confluence, Asana, monday.com, Lattice pricing and the Lattice-Hypercontext relationship.
