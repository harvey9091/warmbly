// What mail clients will do to this body, shown next to the body that causes
// it.
//
// Writing HTML for email is writing for renderers that disagree: Outlook on
// Windows lays out with Word, Gmail clips at 102 KB, and half the clients drop
// a <style> block. The findings come from the server so the editor and the
// send path can never disagree about what ships (internal/pkg/mailhtml).

import React from "react";
import { AlertCircleIcon, AlertTriangleIcon, ChevronDownIcon, InfoIcon } from "lucide-react";
import { AnimatePresence, motion } from "framer-motion";
import type { TemplateHtmlFinding } from "@/lib/api/client/app/campaigns/previewTemplate";

const TONE = {
    error: {
        Icon: AlertCircleIcon,
        text: "text-rose-600",
        chip: "border-rose-200 bg-rose-50 text-rose-700",
    },
    warning: {
        Icon: AlertTriangleIcon,
        text: "text-amber-600",
        chip: "border-amber-200 bg-amber-50 text-amber-700",
    },
    info: {
        Icon: InfoIcon,
        text: "text-slate-400",
        chip: "border-slate-200 bg-slate-50 text-slate-600",
    },
} as const;

const RANK = { error: 0, warning: 1, info: 2 } as const;

export default function HtmlFindings({ findings }: { findings: TemplateHtmlFinding[] }) {
    const [open, setOpen] = React.useState(false);
    if (findings.length === 0) return null;

    const sorted = [...findings].sort((a, b) => RANK[a.severity] - RANK[b.severity]);
    const worst = sorted[0].severity;
    const tone = TONE[worst];
    const counts = {
        error: sorted.filter((f) => f.severity === "error").length,
        warning: sorted.filter((f) => f.severity === "warning").length,
    };

    const summary =
        counts.error > 0
            ? `${counts.error} thing${counts.error === 1 ? "" : "s"} clients will strip or clip`
            : counts.warning > 0
              ? `${counts.warning} thing${counts.warning === 1 ? "" : "s"} to check before sending`
              : "Email client notes";

    return (
        <div className="mt-1.5 rounded-md border border-slate-200 bg-white">
            <button
                type="button"
                onClick={() => setOpen((o) => !o)}
                aria-expanded={open}
                className="flex w-full items-center gap-1.5 px-2.5 py-1.5 text-left"
            >
                <tone.Icon className={`w-3.5 h-3.5 shrink-0 ${tone.text}`} />
                <span className="text-[11.5px] font-medium text-slate-700">{summary}</span>
                <span className="ml-auto flex items-center gap-1.5">
                    <span className="text-[10.5px] text-slate-400">
                        {sorted.length} note{sorted.length === 1 ? "" : "s"}
                    </span>
                    <ChevronDownIcon
                        className={`w-3.5 h-3.5 text-slate-400 transition-transform ${open ? "rotate-180" : ""}`}
                    />
                </span>
            </button>
            <AnimatePresence initial={false}>
                {open && (
                    <motion.ul
                        initial={{ height: 0, opacity: 0 }}
                        animate={{ height: "auto", opacity: 1 }}
                        exit={{ height: 0, opacity: 0 }}
                        transition={{ duration: 0.15 }}
                        className="overflow-hidden border-t border-slate-100"
                    >
                        {sorted.map((f) => {
                            const t = TONE[f.severity];
                            return (
                                <li
                                    key={f.code}
                                    className="flex items-start gap-2 px-2.5 py-2 [&+&]:border-t [&+&]:border-slate-100"
                                >
                                    <span
                                        className={`mt-px shrink-0 rounded border px-1.5 py-0.5 text-[9.5px] uppercase tracking-[0.1em] ${t.chip}`}
                                    >
                                        {f.severity}
                                    </span>
                                    <span className="text-[11.5px] leading-relaxed text-slate-600">{f.message}</span>
                                </li>
                            );
                        })}
                    </motion.ul>
                )}
            </AnimatePresence>
        </div>
    );
}
