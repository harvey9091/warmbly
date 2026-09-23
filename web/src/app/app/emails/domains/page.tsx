// Sending domains: every domain the workspace sends from, its authentication,
// the tracking host its mailboxes use, whether its root redirects to the
// company website, and the inbox vendor that holds it. A row opens the drawer.
import React from "react";
import { Link, useSearchParams } from "react-router-dom";
import { AnimatePresence, motion } from "framer-motion";
import { ArrowLeftIcon, ChevronRightIcon, GlobeIcon, PlusIcon } from "lucide-react";
import { NoAccess } from "@/components/layout/NoAccess";
import { usePermission } from "@/hooks/usePermission";
import { useUserProfile } from "@/hooks/context/user";
import { useSendingDomains } from "@/lib/api/hooks/app/emails/useSendingDomains";
import type { SendingDomain } from "@/lib/api/models/app/emails/SendingDomain";
import type { AppError } from "@/lib/api/client/normalizeError";
import buildError from "@/lib/helper/buildError";
import { SearchInput } from "@/components/ui/field";
import AnimatedNumber from "@/components/ui/AnimatedNumber";
import { Page, PageBody } from "@/components/layout/Page";
import { LogoStack } from "@/components/app/emails/ProviderLogo";
import SendingDomainDrawer, { type DomainTab } from "@/components/app/emails/domains/SendingDomainDrawer";
import { StatusPill, VendorChip } from "@/components/app/emails/domains/parts";
import { authPill, domainStats, domainVendor, needsAttention, redirectPill, trackingPill } from "@/components/app/emails/domains/rules";
import { cn } from "@/lib/utils";

type Filter = "all" | "attention" | "vendor";

const FILTERS: { key: Filter; label: string }[] = [
    { key: "all", label: "All" },
    { key: "attention", label: "Needs attention" },
    { key: "vendor", label: "Vendor managed" },
];

const ROW_GRID =
    "grid grid-cols-[minmax(0,1fr)_auto] md:grid-cols-[minmax(0,1.5fr)_72px_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_16px] items-center gap-x-3";

export default function SendingDomainsPage() {
    const canView = usePermission("MANAGE_EMAILS");
    const profile = useUserProfile();
    const list = useSendingDomains(canView);
    const [query, setQuery] = React.useState("");
    const [filter, setFilter] = React.useState<Filter>("all");
    const [open, setOpen] = React.useState("");
    const [tab, setTab] = React.useState<DomainTab>("overview");
    const [searchParams, setSearchParams] = useSearchParams();

    const domains = list.data?.data;
    const all = React.useMemo(() => domains ?? [], [domains]);
    const stats = React.useMemo(() => domainStats(all), [all]);
    const shown = React.useMemo(() => {
        const q = query.trim().toLowerCase();
        return all.filter((d) => {
            if (filter === "attention" && !needsAttention(d)) return false;
            if (filter === "vendor" && !domainVendor(d)) return false;
            return !q || d.domain.includes(q);
        });
    }, [all, query, filter]);
    const counts: Record<Filter, number> = { all: stats.domains, attention: stats.attention, vendor: stats.vendor };

    // ?domain=<name>&tab=redirect opens that domain on arrival, e.g. from an import's DNS link.
    React.useEffect(() => {
        const d = searchParams.get("domain");
        if (!d) return;
        const t = searchParams.get("tab");
        setTab(t === "redirect" || t === "tracking" ? t : "overview");
        setOpen(d.toLowerCase());
        setSearchParams(
            (prev) => {
                const next = new URLSearchParams(prev);
                next.delete("domain");
                next.delete("tab");
                return next;
            },
            { replace: true },
        );
    }, [searchParams, setSearchParams]);

    const openDomain = (d: string, t: DomainTab = "overview") => {
        setTab(t);
        setOpen(d);
    };
    const addMailboxes = () => profile.setAddEmail(true);

    if (!canView) {
        return <NoAccess feature="sending domains" permissionLabel="Manage mailboxes" />;
    }

    const loading = list.isLoading;
    const empty = !loading && !list.error && all.length === 0;

    return (
        <Page>
            <div className="min-h-12 md:h-12 px-5 py-1.5 md:py-0 border-b border-slate-200 flex flex-wrap items-center gap-3 gap-y-1.5 shrink-0 bg-white sticky top-0 z-10">
                <Link
                    to="/app/emails"
                    className="inline-flex items-center gap-1 h-6 -ml-1.5 px-1.5 rounded-md text-[11.5px] text-slate-500 hover:text-slate-900 hover:bg-slate-100 transition-colors"
                >
                    <ArrowLeftIcon className="w-3 h-3" />
                    Accounts
                </Link>
                <div className="h-4 w-px bg-slate-200" />
                <span className="text-[10px] uppercase tracking-[0.14em] text-slate-400 font-medium">Sending domains</span>
                <span className="text-[12.5px] text-slate-600 truncate hidden lg:inline">
                    Authentication, tracking and redirects for every domain you send from
                </span>
                <button
                    type="button"
                    onClick={addMailboxes}
                    className="ml-auto h-7 px-2.5 rounded-md bg-sky-600 hover:bg-sky-700 text-white text-[12px] font-medium inline-flex items-center gap-1.5 transition-colors"
                >
                    <PlusIcon className="w-3 h-3" />
                    Add mailboxes
                </button>
            </div>

            {!empty && (
                <div className="px-5 py-4 border-b border-slate-200 grid grid-cols-2 md:grid-cols-5 gap-2.5 shrink-0">
                    <StatCard label="Domains" loading={loading} value={stats.domains} sub="sending mail" />
                    <StatCard
                        label="Mailboxes"
                        loading={loading}
                        value={stats.mailboxes}
                        sub={`across ${stats.domains.toLocaleString()} ${stats.domains === 1 ? "domain" : "domains"}`}
                    />
                    <StatCard label="Authenticated" loading={loading} value={stats.authenticated} of={stats.domains} tone="emerald" />
                    <StatCard label="Tracking verified" loading={loading} value={stats.tracking} of={stats.domains} tone="sky" />
                    <StatCard
                        label="Redirects live"
                        loading={loading}
                        value={stats.redirects}
                        of={stats.domains}
                        tone="slate"
                        className="col-span-2 md:col-span-1"
                    />
                </div>
            )}

            {!empty && (
                <div className="px-5 border-b border-slate-200 flex flex-wrap items-center gap-x-3 gap-y-1.5 shrink-0">
                    <div className="flex items-center gap-1 -ml-2.5 overflow-x-auto overflow-y-hidden no-scrollbar" role="tablist" aria-label="Filter domains">
                        {FILTERS.map((f) => {
                            const active = filter === f.key;
                            return (
                                <button
                                    key={f.key}
                                    type="button"
                                    role="tab"
                                    aria-selected={active}
                                    onClick={() => setFilter(f.key)}
                                    className={cn(
                                        "relative h-10 px-2.5 inline-flex shrink-0 items-center gap-1.5 text-[12.5px] transition-colors",
                                        active ? "text-slate-900 font-medium" : "text-slate-500 hover:text-slate-800",
                                    )}
                                >
                                    {f.label}
                                    <span
                                        className={cn(
                                            "min-w-[18px] h-[18px] px-1 rounded-full text-[10.5px] tabular-nums inline-flex items-center justify-center transition-colors",
                                            active
                                                ? f.key === "attention" && counts.attention > 0
                                                    ? "bg-amber-100 text-amber-800"
                                                    : "bg-sky-50 text-sky-700"
                                                : "bg-slate-100 text-slate-500",
                                        )}
                                    >
                                        {loading ? "–" : counts[f.key]}
                                    </span>
                                    {active && (
                                        <motion.span
                                            layoutId="domain-filter-underline"
                                            className="absolute left-1.5 right-1.5 bottom-0 h-0.5 rounded-full bg-sky-600"
                                            transition={{ type: "spring", duration: 0.3, bounce: 0.15 }}
                                        />
                                    )}
                                </button>
                            );
                        })}
                    </div>
                    <SearchInput value={query} onChange={setQuery} placeholder="Search domains…" className="ml-auto w-full sm:w-56 mb-1.5 sm:mb-0" />
                </div>
            )}

            <PageBody>
                {loading ? (
                    <RowsSkeleton />
                ) : list.error ? (
                    <Empty title="Sending domains could not be loaded" body={buildError(list.error as unknown as AppError)} />
                ) : empty ? (
                    <Empty
                        title="No sending domains yet"
                        body="Every domain you connect a mailbox on shows up here, with its authentication, tracking host and website redirect."
                        action={
                            <button
                                type="button"
                                onClick={addMailboxes}
                                className="h-7 px-2.5 rounded-md bg-sky-600 hover:bg-sky-700 text-white text-[12px] font-medium inline-flex items-center gap-1.5 transition-colors"
                            >
                                <PlusIcon className="w-3 h-3" />
                                Add mailboxes
                            </button>
                        }
                    />
                ) : (
                    <>
                        <div className={cn(ROW_GRID, "hidden md:grid px-5 h-8 border-b border-slate-200/60 bg-slate-50/40")}>
                            <Th>Domain</Th>
                            <Th className="text-right">Mailboxes</Th>
                            <Th>Authentication</Th>
                            <Th>Tracking</Th>
                            <Th>Redirect</Th>
                            <span />
                        </div>
                        <motion.div layout="position" className="relative">
                            <AnimatePresence initial={false} mode="popLayout">
                                {shown.map((d) => (
                                    <motion.div
                                        key={d.domain}
                                        layout="position"
                                        initial={{ opacity: 0, y: 4 }}
                                        animate={{ opacity: 1, y: 0 }}
                                        exit={{ opacity: 0, transition: { duration: 0.12 } }}
                                        transition={{ duration: 0.18, ease: [0.22, 1, 0.36, 1] }}
                                    >
                                        <DomainRow d={d} onOpen={openDomain} />
                                    </motion.div>
                                ))}
                                {shown.length === 0 && (
                                    <motion.div
                                        key="none"
                                        layout
                                        initial={{ opacity: 0 }}
                                        animate={{ opacity: 1 }}
                                        exit={{ opacity: 0 }}
                                    >
                                        <Empty
                                            title={query ? "No domain matches" : filter === "attention" ? "Nothing needs attention" : "No vendor-managed domains"}
                                            body={
                                                query
                                                    ? "Try another search."
                                                    : filter === "attention"
                                                      ? "Every domain passes authentication and nothing is waiting for DNS."
                                                      : "Domains whose mailboxes came from an inbox vendor show up here."
                                            }
                                        />
                                    </motion.div>
                                )}
                            </AnimatePresence>
                        </motion.div>
                    </>
                )}
            </PageBody>

            <SendingDomainDrawer domains={domains} open={open} tab={tab} setTab={setTab} onClose={() => setOpen("")} />
        </Page>
    );
}

function Th({ children, className }: { children?: React.ReactNode; className?: string }) {
    return <span className={cn("text-[10px] font-medium text-slate-400 uppercase tracking-[0.14em]", className)}>{children}</span>;
}

const BAR_TONE = { emerald: "bg-emerald-500", sky: "bg-sky-500", slate: "bg-slate-700" } as const;

function StatCard({
    label,
    value,
    of,
    sub,
    tone,
    loading,
    className,
}: {
    label: string;
    value: number;
    /** Shows "of N" and a bar for the share. */
    of?: number;
    sub?: string;
    tone?: keyof typeof BAR_TONE;
    loading: boolean;
    className?: string;
}) {
    const share = of ? Math.min(1, value / of) : 0;
    return (
        <div className={cn("rounded-lg border border-slate-200 bg-white px-3 py-2.5 min-w-0", className)}>
            <div className="text-[10px] uppercase tracking-[0.14em] text-slate-400 font-medium truncate">{label}</div>
            {loading ? (
                <div className="mt-2 h-5 w-12 rounded bg-slate-100 animate-pulse" />
            ) : (
                <div className="mt-1 flex items-baseline gap-1 min-w-0">
                    <span className="text-[22px] text-slate-900 font-light leading-none tabular-nums">
                        <AnimatedNumber value={value} />
                    </span>
                    {of !== undefined && <span className="text-[11px] text-slate-400 tabular-nums truncate">of {of.toLocaleString()}</span>}
                </div>
            )}
            {of !== undefined && tone ? (
                <div className="mt-2 h-1 rounded-full bg-slate-100 overflow-hidden">
                    <motion.div
                        className={cn("h-full rounded-full", BAR_TONE[tone])}
                        initial={false}
                        animate={{ width: `${loading ? 0 : Math.round(share * 100)}%` }}
                        transition={{ duration: 0.5, ease: [0.22, 1, 0.36, 1] }}
                    />
                </div>
            ) : (
                sub && <div className="mt-1.5 text-[10.5px] text-slate-400 truncate">{sub}</div>
            )}
        </div>
    );
}

function DomainRow({ d, onOpen }: { d: SendingDomain; onOpen: (domain: string, tab?: DomainTab) => void }) {
    const auth = authPill(d);
    const tracking = trackingPill(d);
    const redirect = redirectPill(d);
    const hosts = d.mail_hosts ?? [];
    return (
        <div
            role="button"
            tabIndex={0}
            onClick={() => onOpen(d.domain)}
            onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    onOpen(d.domain);
                }
            }}
            className={cn(
                ROW_GRID,
                "group px-5 py-2.5 gap-y-2 border-b border-slate-200/60 bg-white hover:bg-slate-50/80 cursor-pointer transition-colors outline-none focus-visible:bg-slate-50",
            )}
        >
            <div className="flex items-center gap-2.5 min-w-0">
                {hosts.length > 0 ? (
                    <LogoStack ids={hosts} size="md" max={2} className="shrink-0" />
                ) : (
                    <span className="size-6 rounded-md border border-slate-200 bg-white inline-flex items-center justify-center shrink-0">
                        <GlobeIcon className="w-3 h-3 text-slate-400" />
                    </span>
                )}
                <div className="min-w-0">
                    <div className="flex items-center gap-1.5 min-w-0">
                        <span className="text-[12.5px] text-slate-900 font-medium truncate">{d.domain}</span>
                        <VendorChip d={d} className="hidden sm:inline-flex" />
                        <VendorChip d={d} compact className="sm:hidden" />
                    </div>
                    <div className="md:hidden text-[11px] text-slate-500 tabular-nums">
                        {d.mailboxes.toLocaleString()} {d.mailboxes === 1 ? "mailbox" : "mailboxes"}
                    </div>
                </div>
            </div>
            <div className="hidden md:block text-right text-[12px] text-slate-700 tabular-nums">{d.mailboxes.toLocaleString()}</div>
            <div className="hidden md:flex min-w-0">
                <StatusPill tone={auth.tone} title={auth.title} onClick={() => onOpen(d.domain, "overview")}>
                    {auth.label}
                </StatusPill>
            </div>
            <div className="hidden md:flex min-w-0">
                <StatusPill tone={tracking.tone} title={tracking.title} onClick={() => onOpen(d.domain, "tracking")}>
                    {tracking.label}
                </StatusPill>
            </div>
            <div className="hidden md:flex min-w-0">
                <StatusPill tone={redirect.tone} title={redirect.title} onClick={() => onOpen(d.domain, "redirect")}>
                    {redirect.label}
                </StatusPill>
            </div>
            <ChevronRightIcon className="w-3.5 h-3.5 text-slate-300 group-hover:text-slate-500 group-hover:translate-x-0.5 transition-all justify-self-end" />
            <div className="md:hidden col-span-2 flex flex-wrap gap-1.5 min-w-0">
                <StatusPill tone={auth.tone} title={auth.title}>
                    {auth.label}
                </StatusPill>
                <StatusPill tone={tracking.tone} title={tracking.title}>
                    {tracking.tone === "slate" ? "No tracking domain" : tracking.label}
                </StatusPill>
                <StatusPill tone={redirect.tone} title={redirect.title}>
                    {redirect.tone === "slate" ? "No redirect" : redirect.label}
                </StatusPill>
            </div>
        </div>
    );
}

function RowsSkeleton() {
    return (
        <div>
            {Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className={cn(ROW_GRID, "px-5 py-3 border-b border-slate-200/60")}>
                    <div className="flex items-center gap-2.5">
                        <div className="size-6 rounded-md bg-slate-100 animate-pulse" />
                        <div className="h-3 rounded bg-slate-100 animate-pulse" style={{ width: `${40 + ((i * 17) % 35)}%` }} />
                    </div>
                    <div className="hidden md:block h-3 w-6 ml-auto rounded bg-slate-100 animate-pulse" />
                    <div className="hidden md:block h-5 w-24 rounded-full bg-slate-100 animate-pulse" />
                    <div className="hidden md:block h-5 w-28 rounded-full bg-slate-100 animate-pulse" />
                    <div className="hidden md:block h-5 w-20 rounded-full bg-slate-100 animate-pulse" />
                    <span />
                </div>
            ))}
        </div>
    );
}

function Empty({ title, body, action }: { title: string; body: string; action?: React.ReactNode }) {
    return (
        <div className="px-5 py-16 flex flex-col items-center text-center">
            <div className="size-10 rounded-full bg-sky-50 text-sky-600 inline-flex items-center justify-center mb-3">
                <GlobeIcon className="w-4 h-4" />
            </div>
            <p className="text-[13px] text-slate-800 font-medium">{title}</p>
            <p className="mt-1 text-[12px] text-slate-500 max-w-[42ch] leading-relaxed">{body}</p>
            {action && <div className="mt-4">{action}</div>}
        </div>
    );
}
