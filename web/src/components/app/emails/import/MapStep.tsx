// Columns step: what each column holds. The server's reading comes first
// (a saved mapping, a vendor export, a recognised header, a guess from the
// values) and every change re-reads the list, so the counts stay honest.
import { KeyRoundIcon, Loader2Icon, SparklesIcon } from "lucide-react";
import type { ImportField, ImportMapping, MailboxImportPreview } from "@/lib/api/models/app/emails/MailboxImport";
import { SelectMenu } from "@/components/ui/select-menu";
import { Label, TextInput } from "@/components/ui/field";
import { Toggle } from "@/components/app/campaigns/preferences/components/CampaignPreferenceBoolBox";
import { cn } from "@/lib/utils";
import { FIELD_OPTIONS, SOURCE_CHIPS, hasPassword, isRepeatable, plural } from "./importFields";
import { Pill } from "./parts";
import ProviderLogo from "@/components/app/emails/ProviderLogo";

export default function MapStep({
    preview,
    mapping,
    onMapping,
    hasHeader,
    onHasHeader,
    sharedPassword,
    onSharedPassword,
    previewing,
}: {
    preview: MailboxImportPreview;
    mapping: ImportMapping;
    onMapping: (m: ImportMapping) => void;
    hasHeader: boolean;
    onHasHeader: (v: boolean) => void;
    sharedPassword: string;
    onSharedPassword: (v: string) => void;
    previewing: boolean;
}) {
    function setField(index: number, field: ImportField) {
        const next: ImportMapping = { ...mapping };
        // A field that takes one column moves off whichever column held it.
        if (!isRepeatable(field)) {
            for (const [k, v] of Object.entries(next)) if (v === field) next[k] = "ignore";
        }
        next[String(index)] = field;
        onMapping(next);
    }

    const vendor = preview.vendor;
    const s = preview.summary;

    return (
        <div className="p-4 space-y-3">
            {vendor && (
                <div className="rounded-md border border-sky-200 bg-sky-50 px-3 py-2 flex items-center gap-2">
                    {vendor.id ? <ProviderLogo id={vendor.id} size="md" /> : <SparklesIcon className="w-3.5 h-3.5 text-sky-600 shrink-0" />}
                    <span className="text-[12.5px] text-sky-900">
                        Looks like {/^[aeiou]/i.test(vendor.label) ? "an" : "a"} <span className="font-medium">{vendor.label}</span> export.
                        The columns are mapped the way {vendor.label} writes them.
                    </span>
                </div>
            )}

            <div className="flex items-center gap-3">
                <div className="min-w-0 flex-1">
                    <p className="text-[12.5px] text-slate-900 font-medium">What each column holds</p>
                    <p className="text-[11.5px] text-slate-500">
                        {preview.saved_mapping
                            ? "These headers were mapped before, so that mapping is used again."
                            : "Check the suggestions. The mapping is remembered for the next file with the same headers."}
                    </p>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                    <span className="text-[11.5px] text-slate-600">First row is a header</span>
                    <Toggle value={hasHeader} onChange={onHasHeader} ariaLabel="First row is a header" />
                </div>
            </div>

            <div className="rounded-md border border-slate-200 overflow-x-auto">
                <table className="w-full text-left">
                    <thead className="bg-slate-50/60">
                        <tr className="border-b border-slate-200">
                            <th className="px-3 py-2 text-[10px] font-medium text-slate-400 uppercase tracking-[0.14em]">Column</th>
                            <th className="hidden md:table-cell px-3 py-2 text-[10px] font-medium text-slate-400 uppercase tracking-[0.14em]">Values</th>
                            <th className="px-3 py-2 text-[10px] font-medium text-slate-400 uppercase tracking-[0.14em] w-[230px]">Holds</th>
                        </tr>
                    </thead>
                    <tbody>
                        {preview.columns.map((col) => {
                            const field = mapping[String(col.index)] ?? "ignore";
                            const chip = field === col.field && field !== "ignore" ? SOURCE_CHIPS[col.source] : undefined;
                            const chipLabel = col.source === "vendor" ? vendor?.label ?? "Vendor" : chip?.label;
                            const samples = (col.samples ?? []).map((v) => (typeof v === "string" ? v : String(v ?? ""))).filter((v) => v !== "").slice(0, 3);
                            return (
                                <tr key={col.index} className={cn("border-b border-slate-100 last:border-b-0", field === "ignore" && "bg-slate-50/40")}>
                                    <td className="px-3 py-2 align-top">
                                        <div className={cn("text-[12px] font-medium truncate max-w-[180px]", field === "ignore" ? "text-slate-400" : "text-slate-900")}>
                                            {col.header || `Column ${col.index + 1}`}
                                        </div>
                                        <div className="md:hidden text-[11px] text-slate-400 font-mono truncate max-w-[180px]">{samples[0] ?? ""}</div>
                                    </td>
                                    <td className="hidden md:table-cell px-3 py-2 align-top">
                                        {samples.length === 0 ? (
                                            <span className="text-[11.5px] text-slate-300">empty</span>
                                        ) : (
                                            <div className="space-y-0.5">
                                                {samples.map((v, i) => (
                                                    <div key={i} className="text-[11.5px] text-slate-500 font-mono truncate max-w-[240px] inline-flex items-center gap-1 w-full">
                                                        {col.secret && i === 0 && <KeyRoundIcon className="w-3 h-3 text-slate-300 shrink-0" />}
                                                        <span className="truncate">{v}</span>
                                                    </div>
                                                ))}
                                            </div>
                                        )}
                                    </td>
                                    <td className="px-3 py-2 align-top">
                                        <SelectMenu
                                            value={field}
                                            onChange={(v) => setField(col.index, v as ImportField)}
                                            options={FIELD_OPTIONS}
                                            fullWidth
                                            aria-label={`What ${col.header || `column ${col.index + 1}`} holds`}
                                        />
                                        {chip && chipLabel && (
                                            <div className="mt-1">
                                                <Pill className={chip.cls} title={chip.hint}>
                                                    {chipLabel}
                                                </Pill>
                                            </div>
                                        )}
                                    </td>
                                </tr>
                            );
                        })}
                    </tbody>
                </table>
            </div>

            {!hasPassword(mapping) && (
                <div className="rounded-md border border-slate-200 p-3">
                    <Label>One password for every mailbox</Label>
                    <TextInput
                        value={sharedPassword}
                        onChange={onSharedPassword}
                        type="password"
                        autoComplete="new-password"
                        placeholder="No column holds a password"
                        className="w-full"
                    />
                    <p className="mt-1.5 text-[11.5px] text-slate-500 leading-relaxed">
                        Used for every row. Hosts that only take an app password (Google, Yahoo, iCloud) need that instead,
                        and Microsoft 365 rows connect with Microsoft sign-in after the import.
                    </p>
                </div>
            )}

            <div className="flex items-center gap-2 text-[11.5px] min-h-[18px]">
                {previewing ? (
                    <span className="inline-flex items-center gap-1.5 text-slate-500">
                        <Loader2Icon className="w-3 h-3 animate-spin" />
                        Checking the list
                    </span>
                ) : (
                    <span className="text-slate-600">
                        {plural(s.total, "mailbox", "mailboxes")}
                        {s.ready > 0 && <span className="text-emerald-700"> · {s.ready.toLocaleString()} ready</span>}
                        {s.needs_signin > 0 && <span className="text-sky-700"> · {s.needs_signin.toLocaleString()} need sign-in</span>}
                        {s.existing > 0 && <span className="text-slate-500"> · {s.existing.toLocaleString()} already here</span>}
                        {s.invalid > 0 && <span className="text-red-600"> · {s.invalid.toLocaleString()} invalid</span>}
                    </span>
                )}
            </div>
        </div>
    );
}
