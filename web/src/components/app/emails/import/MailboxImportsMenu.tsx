// MailboxImportsMenu: the mailboxes page's way back into a running or recent
// import. Hidden until the workspace has one.
import React from "react";
import { FileSpreadsheetIcon, Loader2Icon } from "lucide-react";
import {
    PopoverMenu,
    PopoverMenuContent,
    PopoverMenuItem,
    PopoverMenuLabel,
    PopoverMenuTrigger,
} from "@/components/ui/popover-menu";
import useMailboxImports from "@/lib/api/hooks/app/emails/useMailboxImports";
import { importDone, type MailboxImport } from "@/lib/api/models/app/emails/MailboxImport";
import timeAgo from "@/lib/helper/timeAgo";
import { cn } from "@/lib/utils";
import { importSourceName } from "./importFields";
import MailboxImportDialog from "./MailboxImportDialog";
import ProviderLogo from "@/components/app/emails/ProviderLogo";

function jobState(job: MailboxImport): { text: string; cls: string } {
    const c = job.counts;
    if (job.status === "running") return { text: `${importDone(c).toLocaleString()}/${job.total.toLocaleString()}`, cls: "text-sky-600" };
    if (job.status === "cancelled") return { text: "Stopped", cls: "text-slate-400" };
    if (c.failed > 0) return { text: `${c.failed.toLocaleString()} failed`, cls: "text-red-600" };
    if (c.needs_signin > 0) return { text: `${c.needs_signin.toLocaleString()} to sign in`, cls: "text-sky-600" };
    return { text: "Done", cls: "text-emerald-600" };
}

export default function MailboxImportsMenu() {
    const imports = useMailboxImports();
    const [openId, setOpenId] = React.useState<string | null>(null);
    const jobs = imports.data?.data ?? [];
    const running = jobs.filter((j) => j.status === "running").length;

    return (
        <>
            {jobs.length > 0 && (
                <PopoverMenu align="end">
                    <PopoverMenuTrigger asChild>
                        <button
                            type="button"
                            className="h-7 px-2.5 rounded-md inline-flex items-center gap-1.5 text-[12px] font-medium transition-colors border border-slate-200 hover:border-slate-300 text-slate-700 hover:text-slate-900 bg-white"
                        >
                            {running > 0 ? (
                                <Loader2Icon className="w-3 h-3 text-sky-600 animate-spin" />
                            ) : (
                                <FileSpreadsheetIcon className="w-3 h-3" />
                            )}
                            Imports
                            {running > 0 && <span className="text-sky-600 tabular-nums">{running}</span>}
                        </button>
                    </PopoverMenuTrigger>
                    <PopoverMenuContent minWidth={300}>
                        <PopoverMenuLabel>Recent imports</PopoverMenuLabel>
                        {jobs.map((job) => {
                            const st = jobState(job);
                            return (
                                <PopoverMenuItem
                                    key={job.id}
                                    onSelect={() => setOpenId(job.id)}
                                    icon={
                                        <span
                                            className={cn(
                                                "block size-1.5 rounded-full",
                                                job.status === "running" ? "bg-sky-500 animate-pulse" : "bg-slate-300",
                                            )}
                                        />
                                    }
                                    trailing={
                                        <span className="inline-flex items-center gap-2 text-[11px]">
                                            <span className={cn("tabular-nums", st.cls)}>{st.text}</span>
                                            <span className="text-slate-400">{timeAgo(job.created_at)}</span>
                                        </span>
                                    }
                                >
                                    <span className="inline-flex items-center gap-1.5 min-w-0">
                                        {job.vendor && <ProviderLogo id={job.vendor} size="xs" framed={false} />}
                                        <span className="truncate">
                                            {importSourceName(job)} · {job.total.toLocaleString()}
                                        </span>
                                    </span>
                                </PopoverMenuItem>
                            );
                        })}
                    </PopoverMenuContent>
                </PopoverMenu>
            )}
            <MailboxImportDialog importId={openId} onClose={() => setOpenId(null)} />
        </>
    );
}
