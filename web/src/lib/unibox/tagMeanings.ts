// What each automatic label means, in one sentence a person can read on hover.
//
// A chip reading "going-cold" or "ball-in-our-court" is only useful to somebody
// who already knows the taxonomy, and nobody does on the first day. The
// wording here mirrors the criteria the classifier is actually given
// (internal/app/inboxtag/policy.go), so the explanation and the rule cannot
// drift into saying different things.
//
// Keyed on the label slug rather than on a stored description, so it needs no
// column and no migration.
// ponytail: a hand-kept map; move it to a `categories.description` column if
// labels a user created ever need their own explanations too.

const TAG_MEANINGS: Record<string, string> = {
    // What the message is. Exactly one of these applies.
    "bounce-hard": "Permanent delivery failure. The address does not exist.",
    "bounce-soft": "Temporary delivery failure: mailbox full, greylisted, or the server was busy.",
    "auto-reply-ooo": "An out-of-office or vacation autoresponder, not a person.",
    "auto-reply-ticket": "An automated \"we got your message\" or ticket receipt.",
    "human-reply": "A real person replying to something you sent.",
    "cold-inbound": "Someone pitching you. Not a reply to your outreach.",
    notification: "An automated notice from a service or platform, not addressed to you personally.",
    internal: "From your own team, or forwarded internally.",

    // What a reply wants. Only ever on a human reply.
    agreed: "They agreed to the partnership, call, or next step.",
    "wants-info": "Open to it, but asking questions before deciding.",
    "wants-pricing": "Asking specifically about price, terms, or commercials.",
    "not-now": "Interested in principle, but says the timing is wrong.",
    "not-interested": "Declined, without asking to be removed.",
    "wrong-person": "Not the right contact, or they named someone else.",
    "opt-out": "Asked to be removed, complained, or threatened. Never chased again.",
    scheduling: "Proposed, confirmed, or moved a specific time to talk.",
    "in-progress": "An update on something already agreed, or their side is done.",
    "question-answered": "They answered something you asked.",
    unclear: "A reply whose intent could not be read from the text.",

    // Signals worth finding a thread by.
    "asks-for-call": "They asked for a call, meeting, or demo.",
    "requests-removal": "They asked to stop receiving email.",
    "legal-threat": "They threatened legal action or named a regulator.",
    "needs-human-judgement": "Answering this one well needs a person to read it.",

    // Follow-up state. Computed from your mailbox, not from a model, and kept
    // in sync as time passes.
    "ball-in-our-court": "They replied and you have not answered for two days or more.",
    "awaiting-reply": "You sent last. Still early, nothing to do yet.",
    "follow-up-due": "You sent last, five days ago, and heard nothing back.",
    "going-cold": "They were interested, then went quiet for ten days. Worth a nudge.",

    // The one that means the system declined to decide.
    "needs-review": "At least one classifier answer was below the confidence floor and needs review.",
};

/**
 * tagMeaning returns the hover explanation for a label, or "" for a label this
 * workspace created itself, which needs no explaining to the person who made it.
 */
export function tagMeaning(title: string): string {
    return TAG_MEANINGS[title.trim().toLowerCase()] ?? "";
}

/** isAutomaticTag reports whether a label is one this system applies. */
export function isAutomaticTag(title: string): boolean {
    return tagMeaning(title) !== "";
}
