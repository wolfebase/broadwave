# Steering notes (read every round)

A reviewer (Claude, for the owner) checks the run about every 25 minutes and writes here. These notes outrank the section 4 order and the priorities in `AGENT_PROMPT.md` section 3, but never the "Never" rules, the TUS scope, or App Review. Newest first. When you act on a note, say so in the commit message; don't edit this file yourself.

## Now

- 2026-09-24 23:10 · **Stop driving the Simulator UI with cliclick, menu-bar shortcuts, and OCR.** Round 1 spent 10+ minutes trying to open Simulator windows through the menu bar. Use what `.cursor/skills/broadwave-apple/SKILL.md` documents: `xcrun simctl launch <udid> com.wolfeup.broadwave -BroadwaveWatch <id>`, deep links (`xcrun simctl openurl <udid> broadwave://watch/<id>`), and `xcrun simctl io <udid> screenshot` (needs no window). If the Stream panel needs a tap to open, add a DEBUG-only launch argument (for example `-BroadwaveStreamPanel`) that opens it, then screenshot. Budget 10 minutes for Apple screenshots; if they still fail, commit PB8 with the web evidence and file the Apple screenshots as a follow-up task line.
- 2026-09-24 23:10 · PB8 has ~900 uncommitted lines. Get it to `make check` green and committed this round; a round that ends with it still uncommitted wastes the work.
- 2026-09-24 22:55 · The iPhone Stream panel showed sync around −49 s. Find out whether that is real drift or a display bug before you tick PB8; the Accept bullets need real numbers.
