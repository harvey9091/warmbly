// Premade inbox views: a handful of questions a pipeline turns on, each
// answered by a set of the automatic labels. A view is a client-side scope
// over the labels the workspace already has (the server filters on
// category_ids with OR semantics), so it needs no table and no migration, and
// it works the moment the labels exist, which is when the workspace is created.
//
// Membership is by label slug, the same identifiers policy.go writes, so the
// view and the classifier cannot disagree about what "hot" means.

import type { UniboxCategoryOverview } from "@/lib/api/models/app/unibox/UniboxOverview";

export type UniboxViewId = "hot" | "needs_reply" | "follow_up" | "declined" | "automated";

export interface UniboxView {
    id: UniboxViewId;
    label: string;
    /** The one-line meaning shown on hover. */
    meaning: string;
    /** Label slugs a thread needs at least one of. */
    labels: string[];
}

export const UNIBOX_VIEWS: UniboxView[] = [
    {
        id: "hot",
        label: "Hot leads",
        meaning: "Replies that agreed, proposed a time, asked for a call or asked about pricing.",
        labels: ["agreed", "scheduling", "asks-for-call", "wants-pricing"],
    },
    {
        id: "needs_reply",
        label: "Needs a reply",
        meaning: "They wrote last and nobody has answered for two days or more.",
        labels: ["ball-in-our-court"],
    },
    {
        id: "follow_up",
        label: "Follow up",
        meaning: "You wrote last and heard nothing, or an interested thread went quiet.",
        labels: ["follow-up-due", "going-cold"],
    },
    {
        id: "declined",
        label: "Declined",
        meaning: "Not interested, wrong person, or asked to be removed. Never chased again.",
        labels: ["not-interested", "wrong-person", "opt-out", "requests-removal"],
    },
    {
        id: "automated",
        label: "Automated",
        meaning: "Bounces, autoresponders and platform notices. Nobody is on the other end.",
        labels: ["bounce-hard", "bounce-soft", "auto-reply-ooo", "auto-reply-ticket", "notification"],
    },
];

export function viewById(id: string | null | undefined): UniboxView | undefined {
    return UNIBOX_VIEWS.find((v) => v.id === id);
}

/** The workspace's category rows that belong to a view, by slug. */
export function viewCategories(view: UniboxView, categories: UniboxCategoryOverview[] | undefined): UniboxCategoryOverview[] {
    if (!categories) return [];
    const wanted = new Set(view.labels);
    return categories.filter((c) => wanted.has(c.title.trim().toLowerCase()));
}

/**
 * The category ids a view resolves to. A view whose labels do not exist yet
 * returns the nil id, which matches nothing, so the list is empty rather than
 * unfiltered.
 */
export function viewCategoryIds(view: UniboxView, categories: UniboxCategoryOverview[] | undefined): string[] {
    const ids = viewCategories(view, categories).map((c) => c.id);
    return ids.length > 0 ? ids : ["00000000-0000-0000-0000-000000000000"];
}
