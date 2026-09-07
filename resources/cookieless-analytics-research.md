# Cookieless analytics for the hosted properties

Research behind issue #352. Everything below was checked against a primary
source, and the source is named. Verified September 2026.

## The question

Understand who visits warmbly.com, which pages and channels bring signups, and
what new customers do in the dashboard, with **no cookie banner**: nothing
stored in the visitor's browser and no consent prompt.

That rules out any tool whose identity model is "write an id into the browser
and read it back". It does not rule out counting visitors, because a visitor
can be counted without being remembered.

## Options considered

| Tool | Storage in the browser | Product funnels | Self-host story | Cost at our size |
|---|---|---|---|---|
| **PostHog Cloud EU, cookieless mode** | none in `cookieless_mode: 'always'` | yes | MIT core, self-hostable | 1M events/month free |
| Plausible Cloud | none | no (page analytics only) | AGPL, self-hostable | paid from the first month |
| Fathom | none | no | closed | paid |
| Umami | none | thin | MIT | free self-hosted, we run the server |
| GA4 | cookies, consent required | yes | none | free |
| Matomo (cookieless config) | none when configured | thin | GPL | free self-hosted |

GA4 is out on the banner requirement alone. Plausible, Fathom and Umami answer
"which page" but not "did this person connect a mailbox and launch a campaign",
which is the half of the question that decides what we build next. Matomo would
work but means running and patching another database.

**Decision: PostHog Cloud EU in cookieless mode**, plus a first-party
attribution record written at signup for anything that has to outlive a day.

## How PostHog cookieless mode actually works

Nothing is written to the browser. The identity is computed on PostHog's
servers as a hash. From `rust/common/cookieless/src/manager.rs` in the PostHog
repository, the hash inputs are:

- the team id
- a **daily-rotated salt**, held for `SALT_TTL_SECONDS` and then deleted
- the IP address
- the user agent
- the **root domain** (eTLD+1) of the host, via `extract_root_domain`

Two consequences matter to us:

1. **The salt is deleted daily**, so the same visitor on two different days is
   two different hashes. There is no persistent identifier, and no way to walk
   one back to a person. This is what makes the no-banner position defensible.
2. **The hash uses the registrable root domain**, so `warmbly.com` and
   `app.warmbly.com` are one visitor and one session within a day, with no
   cross-domain wiring on our side.

### Client configuration

From PostHog's cookieless tracking tutorial:

```javascript
posthog.init("<token>", {
  cookieless_mode: "always",
  api_host: "https://eu.i.posthog.com",
});
```

PostHog's own privacy documentation adds: set `person_profiles: 'never'`, since
"a persistent distinct ID is considered Personal Data under GDPR", which turns
`identify()` into a no-op. `alias` events are dropped at ingestion in this mode.
Cookieless server hash mode has to be enabled in the project settings first.

### Server-side events

A backend event has to join the same hash, or it lands as a separate visitor.
The constants are in `rust/common/cookieless/src/constants.rs`:

```rust
pub const COOKIELESS_SENTINEL_VALUE: &str = "$posthog_cookieless";
pub const COOKIELESS_MODE_FLAG_PROPERTY: &str = "$cookieless_mode";
```

and `nodejs/src/ingestion/common/cookieless/cookieless-manager.ts` reads
`$raw_user_agent`, `$ip` and `$host` off the event to compute it. So a
server-side capture is:

```json
{
  "api_key": "<token>",
  "event": "signup_completed",
  "distinct_id": "$posthog_cookieless",
  "properties": {
    "$cookieless_mode": true,
    "$raw_user_agent": "<the browser's UA>",
    "$ip": "<the browser's IP>",
    "$host": "app.warmbly.com"
  }
}
```

posted to `https://eu.i.posthog.com/i/v0/e/` (PostHog's capture API reference).
The same file shows the ingester **deletes** `$ip` and `$raw_user_agent` from
the event once the hash is computed, so the raw values are not retained.

## The legal position

**This section is engineering notes, not legal advice, and it does not
establish that this deployment may run without a banner. Get a
deployment-specific assessment before relying on it.**

Two separate questions get conflated here, so keep them apart.

**Storing or reading anything on the visitor's device.** This is what the
ePrivacy rules (in France, Article 82) attach consent to. `cookieless_mode:
'always'` writes no cookie, no local storage and no session storage, so there
is nothing stored or read to consent to. That is a claim about the mechanism,
and it is one we can verify ourselves rather than take on trust: see the
acceptance checks in the issue.

**Processing the visitor's IP address and user agent server-side.** This is a
GDPR question and it does not go away because nothing was stored in the
browser. It needs a lawful basis, and whether the resulting hash counts as
personal data is contested rather than settled. PostHog's position is that the
hash cannot be reversed; that is an argument, not a ruling.

The CNIL's audience-measurement exemption is often cited here and it is worth
being precise about what it actually says, because it is narrower than the
shorthand suggests. Its published conditions are that the tool is used for a
purpose *strictly limited* to measuring the audience of the site, produces
*anonymous statistics only*, does not allow a person to be followed across
different sites or apps, and does not lead to the data being cross-referenced
with other processing or passed to third parties. It says **nothing** about
hashing schemes, salt rotation, or IP-plus-user-agent constructions, and it
notes that some audience-measurement offerings fall outside the exemption
regardless of how they are configured.

So: the construction above is *designed* against those conditions — no
cross-site identifier, because the hash is scoped to the registrable root
domain; aggregate output only, because `person_profiles: 'never'` makes
`identify` a no-op; and no other processing to join to. Whether a given
deployment qualifies is a judgement about that deployment, and this document
is not that judgement.

What is clear either way is that acquisition-channel and conversion
measurement are **outside** a "strictly audience measurement" purpose. That is
why the acquisition record below is written up as a deliberate product decision
rather than folded into "analytics": it is kept minimal and first-party,
recorded once at signup as part of the account record, disclosed in the privacy
policy, and it travels with a workspace export and is deleted with the account
like the rest of the customer's data.

## Why a first-party record as well

Cookieless mode buys aggregate truth for a day. It cannot answer "the customer
who upgraded in March came from the deliverability guide in January", because
by design nothing joins those two days.

So the channel is recorded once, at signup, on the organization itself:
`landing_path`, `referrer_host`, and the five UTM fields. Nothing is stored in
the browser before signup — the values ride the query string from the marketing
site to the dashboard's signup page and are read there. Revenue by channel is
then a SQL join against subscriptions, owned by us, and it keeps working if the
analytics provider is blocked, changed or dropped.

## Explicitly out of scope

Session replay, heatmaps, surveys and feature flags each store or record more
than a cookieless pageview and would need their own decision. Session replay in
particular would record mailbox and contact screens.

Self-hosted Warmbly loads none of this: the code path exists in the image, and
without a key it is never initialised. See `docs/content/docs/development/data-control.mdx`.

## Sources

- PostHog, "How to do cookieless tracking with PostHog": <https://posthog.com/tutorials/cookieless-tracking>
- PostHog, "Controlling data collection": <https://posthog.com/docs/privacy/data-collection>
- PostHog, capture API reference: <https://posthog.com/docs/api/capture>
- PostHog source, `rust/common/cookieless/src/constants.rs` and `manager.rs`
- PostHog source, `nodejs/src/ingestion/common/cookieless/cookieless-manager.ts`
- PostHog, "Bot and traffic detection" (on `$raw_user_agent` for server-side capture): <https://posthog.com/docs/web-analytics/bot-detection>
- CNIL, "Cookies : solutions pour les outils de mesure d'audience" (the exemption conditions quoted above): <https://www.cnil.fr/fr/cookies-solutions-pour-les-outils-de-mesure-daudience>
