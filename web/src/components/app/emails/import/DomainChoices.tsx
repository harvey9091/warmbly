// Per-domain choices an import can apply as each mailbox connects: one tracking
// host for the domain, and a redirect of its root to the company website. Both
// are optional; their DNS is finished afterwards on the Sending domains page.
import React from "react";
import { Loader2Icon } from "lucide-react";
import { TextInput } from "@/components/ui/field";
import { CheckSquare } from "@/components/ui/check-square";
import { Chip, CopyButton, SuggestionChip } from "@/components/app/emails/domains/parts";
import { redirectTargetProblem } from "@/components/app/emails/domains/rules";
import { cn } from "@/lib/utils";
import { plural } from "./importFields";
import { SectionLabel } from "./parts";
import { effectivePick, offersChoices, type DomainInfo, type DomainPick, type DomainPicks } from "./domainChoiceRules";

function CheckRow({
    checked,
    onToggle,
    label,
    children,
}: {
    checked: boolean;
    onToggle: () => void;
    label: string;
    children?: React.ReactNode;
}) {
    return (
        <div className="flex items-center gap-2 min-w-0 min-h-7">
            <button
                type="button"
                role="checkbox"
                aria-checked={checked}
                onClick={onToggle}
                className="inline-flex items-center gap-2 shrink-0 text-[11.5px] text-slate-700 hover:text-slate-900 rounded outline-none focus-visible:ring-2 focus-visible:ring-sky-100"
            >
                <CheckSquare checked={checked} />
                {label}
            </button>
            {children}
        </div>
    );
}

/** The two choices for one domain, under that domain's row. */
export function DomainChoiceControls({
    info,
    pick,
    onChange,
    className,
}: {
    info: DomainInfo;
    pick: DomainPick | undefined;
    onChange: (next: DomainPick) => void;
    className?: string;
}) {
    if (!offersChoices(info)) return null;
    const e = effectivePick(info, pick);
    const set = (patch: DomainPick) => onChange({ ...pick, ...patch });
    const problem = e.redirect ? redirectTargetProblem(e.url, info.domain) : null;
    const t = info.tracking;
    return (
        <div className={cn("space-y-0.5", className)}>
            {info.trackingLoading ? (
                <div className="min-h-7 flex items-center gap-1.5 text-[11.5px] text-slate-400">
                    <Loader2Icon className="w-3 h-3 animate-spin" />
                    Checking DNS for a tracking host
                </div>
            ) : t ? (
                <>
                    <CheckRow checked={e.track} onToggle={() => set({ track: !e.track })} label="Tracking domain">
                        <span className="text-[11.5px] font-mono text-slate-600 truncate min-w-0">{t.host}</span>
                        <SuggestionChip status={t.status} />
                    </CheckRow>
                    {e.track && t.status === "suggested" && t.cname_target && (
                        <p className="pl-[22px] text-[11px] text-slate-500 leading-relaxed flex items-center gap-1 flex-wrap">
                            <span>After the import, add a CNAME for</span>
                            <span className="font-mono text-slate-700">{t.host}</span>
                            <span>pointing at</span>
                            <span className="font-mono text-slate-700 break-all">{t.cname_target}</span>
                            <CopyButton value={t.cname_target} label="CNAME value" />
                        </p>
                    )}
                </>
            ) : null}
            <CheckRow checked={e.redirect} onToggle={() => set({ redirect: !e.redirect })} label="Redirect root to">
                {e.redirect ? (
                    <TextInput
                        value={e.url}
                        onChange={(v) => set({ url: v })}
                        placeholder="https://yourcompany.com"
                        invalid={!!problem && e.url.trim() !== ""}
                        title={problem ?? undefined}
                        className="flex-1 min-w-0"
                    />
                ) : e.url ? (
                    <span className="text-[11.5px] text-slate-500 truncate min-w-0">{e.url}</span>
                ) : null}
                {info.redirect?.verified && <Chip tone="emerald">Verified</Chip>}
            </CheckRow>
            {e.redirect && problem && e.url.trim() !== "" && <p className="pl-[22px] text-[11px] text-rose-700">{problem}</p>}
        </div>
    );
}

/** A compact list of the picked mailboxes' domains with their choices, for the vendor and grant wizards. */
export function DomainChoicesSection({
    infos,
    picks,
    setPicks,
    capped = 0,
}: {
    infos: DomainInfo[];
    picks: DomainPicks;
    setPicks: React.Dispatch<React.SetStateAction<DomainPicks>>;
    /** Domains past the ones shown, which get no choices here. */
    capped?: number;
}) {
    const [all, setAll] = React.useState(false);
    const usable = infos.filter(offersChoices);
    if (usable.length === 0) return null;
    const shown = all ? usable : usable.slice(0, 6);
    return (
        <div>
            <SectionLabel className="mb-1">Domains</SectionLabel>
            <p className="text-[11.5px] text-slate-500 leading-relaxed mb-1.5">
                Optional, per domain. Each is applied as its mailboxes connect, and any DNS still to add waits on the Sending domains
                page.
            </p>
            <div className="rounded-md border border-slate-200 divide-y divide-slate-100">
                {shown.map((info) => (
                    <div key={info.domain} className="px-3 py-2">
                        <div className="text-[12.5px] font-medium text-slate-900 truncate">{info.domain}</div>
                        <DomainChoiceControls
                            info={info}
                            pick={picks[info.domain]}
                            onChange={(p) => setPicks((prev) => ({ ...prev, [info.domain]: p }))}
                            className="mt-0.5"
                        />
                    </div>
                ))}
                {usable.length > shown.length && (
                    <button
                        type="button"
                        onClick={() => setAll(true)}
                        className="w-full h-8 text-[11.5px] text-slate-600 hover:text-slate-900 hover:bg-slate-50 transition-colors"
                    >
                        Show all {usable.length.toLocaleString()} domains
                    </button>
                )}
            </div>
            {capped > 0 && (
                <p className="mt-1 text-[11px] text-slate-500">
                    {plural(capped, "more domain is", "more domains are")} not listed. Set them up on the Sending domains page after the
                    import.
                </p>
            )}
        </div>
    );
}
