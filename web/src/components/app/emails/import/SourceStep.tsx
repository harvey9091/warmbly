// Source step: a file dropped or chosen, or a list pasted in. The server reads
// either one; a file moves on by itself once read, a pasted list shows its
// counts live under the box.
import React from "react";
import { FileSpreadsheetIcon, UploadIcon, XIcon } from "lucide-react";
import type { MailboxImportPreview } from "@/lib/api/models/app/emails/MailboxImport";
import useMailboxAllowance from "@/lib/api/hooks/app/emails/useMailboxAllowance";
import { cn } from "@/lib/utils";
import { SectionLabel } from "./parts";
import { plural } from "./importFields";
import { FileAnalyzing, InlineWorking } from "./Discovering";

export default function SourceStep({
    file,
    pasteText,
    onPaste,
    onFile,
    onClearFile,
    preview,
    previewing,
    previewError,
    onAllowance,
}: {
    file: File | null;
    pasteText: string;
    onPaste: (v: string) => void;
    onFile: (f: File) => void;
    onClearFile: () => void;
    preview: MailboxImportPreview | null;
    previewing: boolean;
    previewError: string | null;
    onAllowance?: () => void;
}) {
    const input = React.useRef<HTMLInputElement>(null);
    const [dragging, setDragging] = React.useState(false);
    const allowance = useMailboxAllowance();
    const s = preview?.summary;

    return (
        <div className="p-4 space-y-3">
            <div
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        input.current?.click();
                    }
                }}
                onDragOver={(e) => {
                    e.preventDefault();
                    setDragging(true);
                }}
                onDragLeave={() => setDragging(false)}
                onDrop={(e) => {
                    e.preventDefault();
                    setDragging(false);
                    const f = e.dataTransfer.files?.[0];
                    if (f) onFile(f);
                }}
                onClick={() => input.current?.click()}
                className={cn(
                    "rounded-md border border-dashed p-6 flex flex-col items-center justify-center text-center cursor-pointer transition-colors outline-none focus-visible:ring-2 focus-visible:ring-sky-100",
                    dragging ? "border-sky-400 bg-sky-50" : "border-slate-300 hover:border-slate-400 hover:bg-slate-50/60",
                )}
            >
                <input
                    ref={input}
                    type="file"
                    accept=".csv,.tsv,.txt,.xlsx,text/csv,text/tab-separated-values,text/plain,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
                    className="hidden"
                    onChange={(e) => {
                        const f = e.target.files?.[0];
                        if (f) onFile(f);
                        e.target.value = "";
                    }}
                />
                {file && previewing ? (
                    <FileAnalyzing
                        name={file.name}
                        icon={<FileSpreadsheetIcon className="w-5 h-5 text-emerald-600" />}
                        steps={[
                            "Reading the rows…",
                            "Working out what each column holds…",
                            "Finding each domain's mail servers…",
                            "Checking what is already connected…",
                        ]}
                    />
                ) : file ? (
                    <>
                        <FileSpreadsheetIcon className="w-5 h-5 text-emerald-600" />
                        <p className="text-[13px] font-medium text-slate-900 mt-2">{file.name}</p>
                        <p className="text-[11.5px] text-slate-500 mt-0.5">Drop another file to replace it.</p>
                    </>
                ) : (
                    <>
                        <UploadIcon className="w-5 h-5 text-slate-400" />
                        <p className="text-[13px] font-medium text-slate-900 mt-2">Drop a file here, or click to choose one</p>
                        <p className="text-[11.5px] text-slate-500 mt-0.5">
                            CSV, TSV, TXT or Excel (.xlsx). Any column names: you confirm what each one holds next.
                        </p>
                    </>
                )}
            </div>

            {file && previewError && (
                <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 flex items-start gap-2 text-[11.5px] text-red-800">
                    <span className="flex-1 min-w-0">{previewError}</span>
                    <button type="button" onClick={onClearFile} className="shrink-0 underline font-medium">
                        Choose another
                    </button>
                </div>
            )}
            {file && !previewing && s && s.total === 0 && (
                <p className="text-[11.5px] text-amber-700">No mailboxes found in {file.name}. Check that it has one mailbox per row.</p>
            )}

            <div className="flex items-center gap-3">
                <div className="h-px flex-1 bg-slate-200" />
                <SectionLabel>or paste a list</SectionLabel>
                <div className="h-px flex-1 bg-slate-200" />
            </div>

            <div>
                <div className="relative">
                    <textarea
                        value={pasteText}
                        onChange={(e) => onPaste(e.target.value)}
                        rows={5}
                        spellCheck={false}
                        autoComplete="off"
                        placeholder={file ? "" : "alex@acme.com,password\nsam@acme.com:password\n\nor cells copied from a spreadsheet"}
                        className="w-full rounded-md border border-slate-200 bg-white px-2.5 py-2 text-[16px] md:text-[12.5px] font-mono text-slate-900 placeholder:text-slate-400 outline-none transition-colors focus:border-sky-400 focus:ring-2 focus:ring-sky-100 resize-y min-h-[96px]"
                    />
                    {file && (
                        <p className="absolute inset-x-3 top-2 text-[11.5px] text-slate-400 pointer-events-none">
                            Pasting replaces {file.name}.
                        </p>
                    )}
                </div>
                <div className="min-h-[18px] mt-1 flex items-center gap-2 text-[11.5px]">
                    {!file && pasteText.trim() !== "" && (
                        previewing ? (
                            <InlineWorking>Reading the list and finding each domain's mail servers…</InlineWorking>
                        ) : previewError ? (
                            <span className="text-red-600">{previewError}</span>
                        ) : s ? (
                            <span className="text-slate-600">
                                {plural(s.total, "mailbox", "mailboxes")} found
                                {s.ready > 0 && <span className="text-emerald-700"> · {s.ready.toLocaleString()} ready</span>}
                                {s.needs_signin > 0 && <span className="text-sky-700"> · {s.needs_signin.toLocaleString()} need sign-in</span>}
                                {s.invalid > 0 && <span className="text-red-600"> · {s.invalid.toLocaleString()} invalid</span>}
                            </span>
                        ) : null
                    )}
                    {file && (
                        <button
                            type="button"
                            onClick={onClearFile}
                            className="ml-auto h-6 px-2 rounded text-[11px] text-slate-500 hover:text-slate-900 hover:bg-slate-100 inline-flex items-center gap-1 transition-colors"
                        >
                            <XIcon className="w-3 h-3" />
                            Remove file
                        </button>
                    )}
                </div>
            </div>

            <div className="rounded-md border border-slate-200 bg-slate-50/40 p-3">
                <SectionLabel>What works</SectionLabel>
                <ul className="mt-1.5 text-[11.5px] text-slate-600 leading-relaxed space-y-0.5 list-disc pl-4">
                    <li>One mailbox per row. The email address and its password are enough.</li>
                    <li>SMTP and IMAP settings are detected from each domain. Columns for them win when present.</li>
                    <li>Exports from mailbox vendors are recognised as they are.</li>
                    <li>Google, Yahoo, iCloud and a few others need an app password. The review step says which.</li>
                </ul>
            </div>

            {allowance.data && allowance.data.allowance != null && (
                <p className="text-[11.5px] text-slate-500 leading-relaxed">
                    This workspace holds {allowance.data.used.toLocaleString()} of {allowance.data.allowance.toLocaleString()} mailboxes,
                    so {(allowance.data.remaining ?? 0).toLocaleString()} more fit right now.
                    {onAllowance && (
                        <>
                            {" "}
                            <button type="button" onClick={onAllowance} className="underline text-slate-700 hover:text-slate-900">
                                {(allowance.data.remaining ?? 0) === 0 ? "Request more before importing" : "Need more?"}
                            </button>
                        </>
                    )}
                </p>
            )}
        </div>
    );
}
