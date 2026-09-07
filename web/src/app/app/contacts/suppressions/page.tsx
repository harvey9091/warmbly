// Suppression list: every address and domain campaign mail must not go to,
// however it got there (a click, a reply, a bounce, a complaint, or the
// team). Search, paste-to-add, and per-entry removal. Lifting an entry the
// recipient made themselves is confirmed with a stronger warning.

import React from "react";
import { GlobeIcon, MailIcon, MoreHorizontalIcon, PlusIcon, XIcon } from "lucide-react";
import toast from "react-hot-toast";

import { EmptyBlock, Page, PageBody, PageTopbar, SectionBar, TopbarAction } from "@/components/layout/Page";
import { Label, SearchInput, TextInput } from "@/components/ui/field";
import { Textarea } from "@/components/ui/textarea";
import { PopoverMenu, PopoverMenuContent, PopoverMenuItem, PopoverMenuTrigger } from "@/components/ui/popover-menu";
import { useConfirm } from "@/hooks/context/confirm";
import { useWriteGuard } from "@/hooks/usePermission";
import { useAddSuppressions, useRemoveSuppression, useSuppressions } from "@/lib/api/hooks/app/suppressions/useSuppressions";
import { SOURCE_LABEL, recipientTriggered, type default as Suppression } from "@/lib/api/models/app/suppressions/Suppression";
import type { AppError } from "@/lib/api/client/normalizeError";
import buildError from "@/lib/helper/buildError";
import { fmtAbsolute } from "@/components/app/contacts/contact-edit/format";

export default function SuppressionsPage() {
    const write = useWriteGuard("MANAGE_CONTACTS");
    const guarded = (fn: () => void) => () => write.guard(fn)({});
    const [query, setQuery] = React.useState("");
    const [debounced, setDebounced] = React.useState("");
    const [adding, setAdding] = React.useState(false);
    React.useEffect(() => {
        const t = setTimeout(() => setDebounced(query.trim()), 200);
        return () => clearTimeout(t);
    }, [query]);

    const list = useSuppressions(debounced);

    return (
        <Page>
            <PageTopbar eyebrow="Suppression list" subtitle="Addresses and domains no campaign will email">
                <TopbarAction icon={<PlusIcon className="w-3 h-3" />} onClick={guarded(() => setAdding(true))}>
                    Add
                </TopbarAction>
            </PageTopbar>

            <SectionBar label="Entries" count={list.entries.length + (list.hasNextPage ? "+" : "")}>
                <SearchInput value={query} onChange={setQuery} placeholder="Search addresses or domains…" className="w-full sm:w-72" />
            </SectionBar>

            <PageBody>
                {adding && <AddForm onClose={() => setAdding(false)} />}
                {list.isLoading ? (
                    <div className="px-5 py-3 space-y-2">
                        {[0, 1, 2].map((i) => (
                            <div key={i} className="h-7 rounded bg-slate-100 animate-pulse" />
                        ))}
                    </div>
                ) : list.entries.length === 0 ? (
                    <EmptyBlock
                        title={debounced ? "No entries match" : "Nothing suppressed yet"}
                        body={
                            debounced
                                ? "Try a different search."
                                : "Recipients who unsubscribe, reply asking to stop, bounce or complain land here automatically. Add customers, partners or whole domains by hand so no campaign reaches them."
                        }
                        cta={
                            debounced ? undefined : (
                                <TopbarAction icon={<PlusIcon className="w-3 h-3" />} onClick={guarded(() => setAdding(true))}>
                                    Add addresses
                                </TopbarAction>
                            )
                        }
                    />
                ) : (
                    <div>
                        {list.entries.map((s) => (
                            <SuppressionRow key={s.id} entry={s} />
                        ))}
                        {list.hasNextPage && (
                            <div className="px-5 py-3">
                                <button
                                    type="button"
                                    onClick={() => list.fetchNextPage()}
                                    disabled={list.isFetchingNextPage}
                                    className="h-7 px-2.5 rounded-md border border-slate-200 text-[12px] text-slate-700 hover:border-slate-300 hover:text-slate-900 transition-colors disabled:opacity-50"
                                >
                                    {list.isFetchingNextPage ? "Loading…" : "Load more"}
                                </button>
                            </div>
                        )}
                    </div>
                )}
            </PageBody>
        </Page>
    );
}

// Paste-to-add: one address or domain per line, comma or whitespace also
// accepted, so a column copied out of a spreadsheet works as is.
function AddForm({ onClose }: { onClose: () => void }) {
    const add = useAddSuppressions();
    const [raw, setRaw] = React.useState("");
    const [reason, setReason] = React.useState("");
    const values = React.useMemo(
        () => Array.from(new Set(raw.split(/[\s,;]+/).map((v) => v.trim()).filter(Boolean))),
        [raw],
    );

    async function submit() {
        if (values.length === 0 || add.isPending) return;
        try {
            const res = await add.mutateAsync({ entries: values.map((value) => ({ value })), reason: reason.trim() || undefined });
            const skipped = res.skipped?.length ?? 0;
            toast.success(
                skipped
                    ? `Added ${res.added}; skipped ${skipped} that did not look like an address or domain`
                    : `Added ${res.added} to the suppression list`,
            );
            onClose();
        } catch (err) {
            toast.error(buildError(err as AppError));
        }
    }

    return (
        <form
            onSubmit={(e) => {
                e.preventDefault();
                void submit();
            }}
            className="px-5 py-3 border-b border-slate-200/60 bg-sky-50/40 space-y-2.5"
        >
            <div>
                <Label>Addresses or domains</Label>
                <Textarea
                    value={raw}
                    onChange={(e) => setRaw(e.target.value)}
                    rows={4}
                    autoFocus
                    placeholder={"jane@acme.com\nacme.com\n@partner.io"}
                    className="w-full max-w-[520px] text-[12.5px] font-mono"
                />
                <p className="mt-1 text-[11px] text-slate-500">
                    One per line, or paste a column. A bare domain suppresses every address at it.
                </p>
            </div>
            <div>
                <Label>Reason (optional)</Label>
                <TextInput value={reason} onChange={setReason} placeholder="Existing customer" className="w-full max-w-[520px]" />
            </div>
            <div className="flex items-center gap-2">
                <button
                    type="submit"
                    disabled={values.length === 0 || add.isPending}
                    className="h-7 px-2.5 rounded-md bg-sky-600 hover:bg-sky-700 text-white text-[12px] font-medium transition-colors disabled:opacity-50"
                >
                    {add.isPending ? "Adding…" : values.length > 1 ? `Add ${values.length}` : "Add"}
                </button>
                <button
                    type="button"
                    onClick={onClose}
                    className="h-7 px-2.5 rounded-md text-[12px] text-slate-600 hover:text-slate-900 hover:bg-slate-100 transition-colors"
                >
                    Cancel
                </button>
            </div>
        </form>
    );
}

function SuppressionRow({ entry }: { entry: Suppression }) {
    const confirm = useConfirm();
    const write = useWriteGuard("MANAGE_CONTACTS");
    const remove = useRemoveSuppression();
    const isDomain = entry.kind === "domain";

    function askRemove() {
        const text = recipientTriggered(entry)
            ? `${entry.email} ${SOURCE_LABEL[entry.source]?.toLowerCase() ?? "was suppressed"} on their own. Removing the entry lets campaigns email them again, and that is recorded in the audit log. Continue?`
            : `Remove ${entry.email} from the suppression list? Campaigns can email ${isDomain ? "addresses at this domain" : "this address"} again.`;
        confirm.show(text, async () => {
            try {
                await remove.mutateAsync(entry.id);
                toast.success(`Removed ${entry.email}`);
            } catch (err) {
                toast.error(buildError(err as AppError));
            }
        });
    }

    const Icon = isDomain ? GlobeIcon : MailIcon;
    return (
        <div className="group h-11 px-5 flex items-center gap-3 border-b border-slate-200/60 transition-colors hover:bg-slate-50/80">
            <Icon className="w-3.5 h-3.5 text-slate-400 shrink-0" />
            <div className="min-w-0 flex-1">
                <div className="text-[12.5px] font-medium text-slate-900 truncate">
                    {isDomain ? `@${entry.email}` : entry.email}
                </div>
                <div className="text-[11px] text-slate-500 truncate">{entry.reason || "No reason given"}</div>
            </div>
            <span
                className={`hidden sm:inline-flex h-5 items-center px-1.5 rounded text-[10px] font-medium border shrink-0 ${
                    recipientTriggered(entry)
                        ? "text-red-700 bg-red-50 border-red-200"
                        : "text-slate-600 bg-slate-100 border-slate-200"
                }`}
            >
                {SOURCE_LABEL[entry.source] ?? entry.source}
            </span>
            <span className="hidden md:inline text-[11px] text-slate-400 tabular-nums w-28 text-right shrink-0">
                {fmtAbsolute(entry.created_at)}
            </span>
            <PopoverMenu align="end">
                <PopoverMenuTrigger asChild>
                    <button
                        type="button"
                        aria-label="More"
                        className="size-7 rounded-md text-slate-400 hover:text-slate-900 hover:bg-slate-100 inline-flex items-center justify-center transition-colors shrink-0"
                    >
                        <MoreHorizontalIcon className="w-3.5 h-3.5" />
                    </button>
                </PopoverMenuTrigger>
                <PopoverMenuContent minWidth={200}>
                    <PopoverMenuItem onSelect={() => write.guard(askRemove)({})} danger>
                        <XIcon className="w-3.5 h-3.5" />
                        Remove from list
                    </PopoverMenuItem>
                </PopoverMenuContent>
            </PopoverMenu>
        </div>
    );
}
