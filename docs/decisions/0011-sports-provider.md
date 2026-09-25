# 0011: Sports provider

Accepted, 2026-09-25.

## Goal

Scores for the twelve leagues, from a source a stranger is allowed to ship. Until that source is chosen, the board that is already wired keeps working.

## What the terms say

Read on 2026-09-25. Nothing below is a grant to ship this app's scoreboard. Where a page did not state a rule, it is not stated here.

**ESPN, still the client.** `NewESPN` reads `https://site.api.espn.com/apis/site/v2/sports`. ESPN does not publish a developer license for that host. Disney's terms, last updated 24 May 2024, cover products branded ESPN (`https://disneytermsofuse.com/english/`). The consumer license is personal and noncommercial, with no right to reproduce or distribute the product (section 2.A). Without written permission you may not use the products for a commercial or business use (2.B.viii, 3.H) or access, copy, or extract them with a script or other automated means, including to build a database (2.B.x). The products are for personal, noncommercial use (3.G).

**TheSportsDB.** Terms last updated 17 September 2026 (`https://www.thesportsdb.com/docs_terms_of_use.php`). Free use is "lookup data and artwork for your development projects." The same paragraph says "You cannot publish apps to an appstore unless you are a paid subscriber." Paid use may "develop apps and services as long as you stay within the rate limit," must name TheSportsDB as the source, and "does not grant rights to third-party artwork." Reselling the API needs specific permission. The pricing page (`https://www.thesportsdb.com/docs_pricing.php`) lists "2 min livescore (Soccer, NFL, NBA, MLB, NHL)" on the $9 and $20 tiers, not on the free tier. The free tier would not cover this App Store app, and a paid key is the operator's subscription. It is not in the binary.

**League sites.** NBA.com terms (`https://www.nba.com/termsofuse`, section 9) limit NBA statistics to legitimate news reporting or private, non-commercial use. They bar use in a fantasy game or other commercial product, in a live or near-live play-by-play, and in a product that keeps a comprehensive, regularly updated statistics database, unless the operator consents. Section 1 limits other material from the services to personal, noncommercial use. MLB.com terms (`https://www.mlb.com/official-information/terms-of-use`) limit the MLB digital properties to private, non-commercial use and say not to reproduce or distribute them without written permission. NHL.com terms, last updated 29 October 2025 (`https://www.nhl.com/info/terms-of-service`, section 2), forbid unauthorized spidering, scraping, harvesting, or other automated means to compile information. No public developer terms were found that license a stranger to ship NHL, MLB, or NBA scores inside this app.

**NFL official data.** Sports Business Journal reported on 11 June 2025 that the NFL extended its official data agreement with Genius Sports through the 2029 season, with Genius still the exclusive provider of the league's real-time official data (play-by-play, betting data, and Next Gen Stats) (`https://www.sportsbusinessjournal.com/Articles/2025/06/11/nfl-genius-extend-data-deal-through-2029-season/`). The Genius developer centre (`https://developer.geniussports.com/`) is a client portal. It is not an open license, and this repo has no Genius contract.

**football-data.org.** The coverage page lists twelve soccer competitions as free (`https://www.football-data.org/coverage`). The FAQ asks for the line "Data provided by football-data.org" (`https://www.football-data.org/documentation/faq`). That set is not these twelve leagues, and no current terms were found that clearly allow this product to ship those scores. No client was added.

Sportradar and Stats Perform sell licensed feeds under a contract. No such contract is in the repo. No second client was added.

## Decision

`server/internal/sports` has a registry. `Open("")` and `Open("espn")` return the ESPN provider. `Open("thesportsdb")` returns a client that does not dial until it has a key. `NewESPN` and `NewCache` are unchanged. The process still starts with `NewCache(NewESPN())`.

The owner delegated the default to the reviewer on 2026-09-25. ESPN stays. These conditions are part of the decision:

1. Only the user's server fetches scores. The apps do not. The cache still waits 30 seconds while a game is on and two hours when the board is quiet.
2. Settings > Sports says where the scores come from. Live scores are on until the user turns them off, and off makes no scoreboard request.
3. TheSportsDB is optional. It runs only with a key the user types. No key is in the binary, and nothing is bought. The settings line names TheSportsDB while that key is in use.
4. Team marks in the apps are names and colors. A logo URL from another site is dropped before it is stored or sent. A logo this server hosts (a path with no host) can stay.
5. PRIVACY.md says the request is the server's, and that turning live scores off stops it. The apps still collect nothing for Wolfe Up.

## Consequences

Scores stay on the ESPN board the server already caches. A saved TheSportsDB key selects that provider instead, and clearing the key returns to ESPN. Remote team logos are not shown.
