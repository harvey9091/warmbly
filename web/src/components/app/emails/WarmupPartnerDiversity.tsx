import { cn } from "@/lib/utils";

export interface WarmupPartnerDiversityInfo {
    partner_mailboxes_7d?: number;
    partner_domains_7d?: number;
    partner_organizations_7d?: number;
}

export default function WarmupPartnerDiversity({ health, className }: { health: WarmupPartnerDiversityInfo; className?: string }) {
    const mailboxes = health.partner_mailboxes_7d ?? 0;
    const domains = health.partner_domains_7d ?? 0;
    const organizations = health.partner_organizations_7d ?? 0;
    if (mailboxes <= 0) return null;

    return (
        <div className={cn("mt-2.5", className)}>
            <div className="flex items-center gap-4 text-[11.5px] text-slate-500">
                <span>
                    <b className="text-slate-900 tabular-nums">{mailboxes}</b> partner{mailboxes === 1 ? "" : "s"}
                </span>
                <span>
                    <b className="text-slate-900 tabular-nums">{domains}</b> domain{domains === 1 ? "" : "s"}
                </span>
                <span>
                    <b className="text-slate-900 tabular-nums">{organizations}</b> workspace{organizations === 1 ? "" : "s"}
                </span>
                <span className="text-slate-400">last 7 days</span>
            </div>
            {mailboxes > 1 && organizations === 1 && (
                <p className="mt-1.5 text-[11px] text-amber-700">
                    Every partner this week was in a single workspace. Warmup builds the most reputation across many workspaces and domains.
                </p>
            )}
        </div>
    );
}
