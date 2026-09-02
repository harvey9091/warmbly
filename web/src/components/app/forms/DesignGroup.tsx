// DesignGroup — one collapsible group in the Design rail. The rail carries
// seven groups now, which is more than fits on screen, so each collapses to
// its header and the open set is remembered per form for the session.
//
// Animation follows the disclosure pattern used by the advisor cards: height
// and opacity together, 0.18s ease-out, overflow hidden while it moves.

import React from "react";
import { AnimatePresence, motion } from "framer-motion";
import { ChevronRightIcon } from "lucide-react";

export default function DesignGroup({
    title,
    /** A short summary of the current value, shown while collapsed. */
    hint,
    open,
    onToggle,
    children,
}: {
    title: string;
    hint?: string;
    open: boolean;
    onToggle: () => void;
    children: React.ReactNode;
}) {
    return (
        <section className="border-b border-slate-200 last:border-b-0">
            <button
                type="button"
                onClick={onToggle}
                aria-expanded={open}
                className="w-full h-10 px-4 flex items-center gap-2 text-left hover:bg-slate-50 transition-colors"
            >
                <motion.span
                    animate={{ rotate: open ? 90 : 0 }}
                    transition={{ duration: 0.18, ease: "easeOut" }}
                    className="shrink-0 text-slate-400"
                >
                    <ChevronRightIcon className="w-3.5 h-3.5" />
                </motion.span>
                <span className="text-[10px] uppercase tracking-[0.14em] text-slate-500 font-medium">{title}</span>
                {!open && hint && (
                    <span className="ml-auto text-[11px] text-slate-400 truncate max-w-[45%]">{hint}</span>
                )}
            </button>
            <AnimatePresence initial={false}>
                {open && (
                    <motion.div
                        initial={{ height: 0, opacity: 0 }}
                        animate={{ height: "auto", opacity: 1 }}
                        exit={{ height: 0, opacity: 0 }}
                        transition={{ duration: 0.18, ease: "easeOut" }}
                        className="overflow-hidden"
                    >
                        <div className="px-4 pb-4 flex flex-col gap-3">{children}</div>
                    </motion.div>
                )}
            </AnimatePresence>
        </section>
    );
}
