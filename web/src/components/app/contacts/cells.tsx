// Cell content shared by the contacts table's columns: the subscription pill,
// the per-lead processing pill, and the engagement counts of the Leads view.
// They live apart from the table so the column registry can render them
// without importing the table that renders the registry.

import { AlertTriangleIcon, BanIcon, CheckIcon, ClockIcon, CornerUpLeftIcon, InfoIcon, PauseIcon, type LucideIcon } from "lucide-react";
import clippedTitle from "@/lib/helper/clippedTitle";
import { holdSummary } from "@/lib/api/models/app/contacts/Contact";
import type { ContactCampaignProgress, LeadStatus } from "@/lib/api/models/app/contacts/Contact";

// The empty-value mark every optional column shares.
export function Dash() {
    return <span className="text-slate-300">—</span>;
}

// A header with an info tooltip beside the label.
export function InfoHeader({ label, title, aria }: { label: string; title: string; aria: string }) {
    return (
        <span className="inline-flex items-center gap-1">
            {label}
            <span className="inline-flex cursor-help text-slate-300 hover:text-slate-500" title={title}>
                <InfoIcon className="w-3 h-3" aria-label={aria} />
            </span>
        </span>
    );
}

export function StatusPill({ subscribed }: { subscribed: boolean }) {
    const label = subscribed ? "subscribed" : "unsubscribed";
    return (
        <span
            className={`inline-flex items-center gap-1 max-w-full text-[10.5px] font-medium uppercase tracking-[0.08em] ${
                subscribed ? "text-emerald-700" : "text-slate-500"
            }`}
        >
            <span
                className={`size-1.5 shrink-0 rounded-full ${subscribed ? "bg-emerald-500" : "bg-slate-300"}`}
            />
            {/* The dot carries the state on its own below sm, so the word stays
                for screen readers at every width and the visible copy is the
                one that comes and goes. */}
            <span className="sr-only">{label}</span>
            <span aria-hidden className="hidden sm:inline truncate" {...clippedTitle}>
                {label}
            </span>
        </span>
    );
}

// One engagement count of the Leads view. A count of steps engaged, a dash
// for a lead that was sent but never did, and blank for a lead never emailed.
// A machine-only open (Apple MPP prefetch) reads "auto" so it is not mistaken
// for a person.
export function EngagementValue({
    n,
    sent,
    Icon,
    label,
    auto = false,
}: {
    n: number;
    sent: boolean;
    Icon: LucideIcon;
    label: string;
    auto?: boolean;
}) {
    if (n > 0) {
        return (
            <span
                className="inline-flex items-center gap-1 text-[11px] font-medium text-emerald-700 tabular-nums"
                title={`${label} ${n} ${n === 1 ? "email" : "emails"}`}
            >
                <Icon className="w-3 h-3 shrink-0" />
                {n}
            </span>
        );
    }
    if (auto) {
        return (
            <span
                className="text-[10.5px] text-slate-400"
                title="Opened by a mail client automatically, not by a person"
            >
                auto
            </span>
        );
    }
    if (sent) {
        return (
            <span className="text-slate-300 text-[11px]" aria-label={`not ${label}`}>
                —
            </span>
        );
    }
    return null;
}

// Per-lead processing state inside a campaign (campaign Leads view only).
// `active` renders the animated dot-grid loader (the same "processing" motif
// used across the app); every other state is a distinct lucide icon.
const LEAD_META: Record<
    LeadStatus,
    { label: string; dot: string; text: string; Icon: LucideIcon }
> = {
    pending: { label: "Queued", dot: "bg-slate-300", text: "text-slate-500", Icon: ClockIcon },
    active: { label: "Processing", dot: "bg-sky-500", text: "text-sky-700", Icon: ClockIcon },
    completed: { label: "Done", dot: "bg-indigo-500", text: "text-indigo-700", Icon: CheckIcon },
    replied: { label: "Replied", dot: "bg-emerald-500", text: "text-emerald-700", Icon: CornerUpLeftIcon },
    bounced: { label: "Bounced", dot: "bg-rose-500", text: "text-rose-600", Icon: AlertTriangleIcon },
    failed: { label: "Failed", dot: "bg-rose-500", text: "text-rose-600", Icon: AlertTriangleIcon },
    unsubscribed: { label: "Unsubscribed", dot: "bg-slate-300", text: "text-slate-400", Icon: BanIcon },
    paused: { label: "Paused", dot: "bg-violet-400", text: "text-violet-600", Icon: PauseIcon },
    undeliverable: { label: "Undeliverable", dot: "bg-amber-500", text: "text-amber-600", Icon: AlertTriangleIcon },
};

export function LeadStatusPill({ lead }: { lead?: ContactCampaignProgress | null }) {
    const status: LeadStatus = lead?.status ?? "pending";
    const meta = LEAD_META[status];
    const Icon = meta.Icon;
    // A failed lead carries the worker's reason; surface it on hover since the
    // pill itself only has room for the word.
    const title =
        status === "failed" && lead?.failure_reason
            ? `Could not send: ${lead.failure_reason}`
            : status === "undeliverable"
                ? "Address verification refused this recipient, so the campaign skips it"
                : lead?.hold
                    ? holdSummary(lead.hold)
                    : undefined;
    return (
        <span
            className={`inline-flex items-center gap-1.5 max-w-full text-[10.5px] font-medium uppercase tracking-[0.08em] ${meta.text}`}
            title={title}
        >
            {status === "active" ? (
                <span className="campaign-grid text-sky-600 shrink-0" aria-hidden />
            ) : (
                <Icon className="w-3 h-3 shrink-0" />
            )}
            <span className="sr-only">{meta.label}</span>
            {/* The reason, when there is one, is worth more than the word it
                covers — and React owning the attribute is what clears any word
                the tooltip helper left here before the lead changed state. */}
            <span aria-hidden className="hidden sm:inline truncate" title={title} {...(title ? {} : clippedTitle)}>
                {meta.label}
            </span>
        </span>
    );
}
