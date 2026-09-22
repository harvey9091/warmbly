import { cn } from "@/lib/utils";

export interface WarmupPartnerDiversityInfo {
    partner_mailboxes_7d?: number;
    partner_domains_7d?: number;
    partner_organizations_7d?: number;
    received_7d?: number;
    senders_7d?: number;
}

export default function WarmupPartnerDiversity({ health, className }: { health: WarmupPartnerDiversityInfo; className?: string }) {
    const mailboxes = health.partner_mailboxes_7d ?? 0;
    const domains = health.partner_domains_7d ?? 0;
    const organizations = health.partner_organizations_7d ?? 0;
    const received = health.received_7d ?? 0;
    const senders = health.senders_7d ?? 0;
    if (mailboxes <= 0 && received <= 0) return null;

    // An older cloud answers without the receiving side; only judge it when it is there.
    const knowsReceived = health.received_7d !== undefined;
    const starving = knowsReceived && mailboxes >= 5 && received * 4 < mailboxes;

    return (
        <div className={cn("mt-2.5", className)}>
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[11.5px] text-slate-500">
                <span>
                    Sent to <b className="text-slate-900 tabular-nums">{mailboxes}</b> partner{mailboxes === 1 ? "" : "s"}
                </span>
                <span>
                    <b className="text-slate-900 tabular-nums">{domains}</b> domain{domains === 1 ? "" : "s"}
                </span>
                <span>
                    <b className="text-slate-900 tabular-nums">{organizations}</b> workspace{organizations === 1 ? "" : "s"}
                </span>
                {knowsReceived && (
                    <span>
                        Received <b className="text-slate-900 tabular-nums">{received}</b> from <b className="text-slate-900 tabular-nums">{senders}</b> partner{senders === 1 ? "" : "s"}
                    </span>
                )}
                <span className="text-slate-400">last 7 days</span>
            </div>
            {mailboxes > 1 && organizations === 1 && (
                <p className="mt-1.5 text-[11px] text-amber-700">
                    Every partner this week was in a single workspace. Warmup builds the most reputation across many workspaces and domains.
                </p>
            )}
            {starving && (
                <p className="mt-1.5 text-[11px] text-amber-700">
                    This mailbox is writing to far more partners than are writing back. The pool now favours it on every draw, so the numbers should even out over the next few days.
                </p>
            )}
        </div>
    );
}
