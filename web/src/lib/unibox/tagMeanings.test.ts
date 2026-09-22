// The meanings are what a person reads on hover to find out what a label is
// for. If the taxonomy grows and this map does not, the new label ships with no
// explanation, which is the state the feature started in and the reason this
// file exists.

import { describe, it, expect } from "vitest";
import { tagMeaning, isAutomaticTag } from "./tagMeanings";

// Mirrors AllLabels() in internal/app/inboxtag/policy.go. Kept by hand, which
// is exactly why it is asserted rather than trusted.
const EVERY_AUTOMATIC_LABEL = [
    "agreed", "auto-reply-ooo", "auto-reply-ticket", "awaiting-reply",
    "ball-in-our-court", "bounce-hard", "bounce-soft", "cold-inbound",
    "follow-up-due", "going-cold", "human-reply", "in-progress", "internal",
    "legal-threat", "needs-human-judgement", "needs-review", "not-interested",
    "not-now", "notification", "opt-out", "question-answered",
    "requests-removal", "scheduling", "unclear", "wants-info", "wants-pricing",
    "wrong-person", "asks-for-call",
];

describe("tag meanings", () => {
    it("explains every label the system can apply", () => {
        const missing = EVERY_AUTOMATIC_LABEL.filter((l) => !tagMeaning(l));
        expect(missing).toEqual([]);
    });

    it("writes a sentence, not a restatement of the slug", () => {
        for (const label of EVERY_AUTOMATIC_LABEL) {
            const meaning = tagMeaning(label);
            expect(meaning.length).toBeGreaterThan(20);
            // A "meaning" that just spells the slug back teaches nobody anything.
            expect(meaning.toLowerCase()).not.toBe(label.replace(/-/g, " "));
        }
    });

    it("leaves a workspace's own labels alone", () => {
        expect(tagMeaning("Important")).toBe("");
        expect(isAutomaticTag("Important")).toBe(false);
        expect(isAutomaticTag("going-cold")).toBe(true);
    });

    it("is case and whitespace tolerant, because a title is user-editable", () => {
        expect(tagMeaning("  Going-Cold ")).toBe(tagMeaning("going-cold"));
    });
});
