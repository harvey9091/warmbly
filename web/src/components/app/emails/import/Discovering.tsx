// Loading states for the import screens: a "finding your mailboxes" view that
// says what is happening in steps over skeleton rows shaped like the result,
// skeleton cards for account lists, and a bar for a refresh that keeps the old rows.
import React from "react";
import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import ProviderLogo from "@/components/app/emails/ProviderLogo";
import { cn } from "@/lib/utils";

/** Advances through steps and holds on the last, so a slow answer never loops back to the start. */
function useStep(count: number, every = 1600) {
    const [i, setI] = React.useState(0);
    React.useEffect(() => {
        if (count <= 1) return;
        const t = window.setInterval(() => setI((v) => Math.min(v + 1, count - 1)), every);
        return () => window.clearInterval(t);
    }, [count, every]);
    return i;
}

function Bar({ className }: { className?: string }) {
    return <span className={cn("block rounded skeleton-shimmer", className)} />;
}

/** A pulsing logo and the step in progress, above skeleton rows. */
export function Discovering({
    logo,
    icon,
    steps,
    rows = 6,
    variant = "table",
    className,
}: {
    /** A vendor id, mail host or "google" / "microsoft". */
    logo?: string;
    icon?: React.ReactNode;
    steps: string[];
    rows?: number;
    variant?: "table" | "cards";
    className?: string;
}) {
    const reduce = useReducedMotion();
    const step = useStep(steps.length);
    return (
        <div className={cn("rounded-md border border-slate-200 overflow-hidden", className)} role="status" aria-live="polite">
            <div className="px-4 py-5 flex flex-col items-center text-center gap-3 border-b border-slate-100 bg-gradient-to-b from-sky-50/50 to-white">
                <div className="relative size-11 flex items-center justify-center">
                    {!reduce && (
                        <>
                            <motion.span
                                className="absolute inset-0 rounded-full bg-sky-200/50"
                                animate={{ scale: [1, 1.55], opacity: [0.55, 0] }}
                                transition={{ duration: 1.6, repeat: Infinity, ease: "easeOut" }}
                            />
                            <motion.span
                                className="absolute inset-0 rounded-full bg-sky-200/50"
                                animate={{ scale: [1, 1.55], opacity: [0.55, 0] }}
                                transition={{ duration: 1.6, repeat: Infinity, ease: "easeOut", delay: 0.8 }}
                            />
                        </>
                    )}
                    <span className="relative size-11 rounded-full bg-white border border-slate-200 shadow-sm flex items-center justify-center">
                        {logo ? <ProviderLogo id={logo} size="md" framed={false} title="" /> : icon}
                    </span>
                </div>
                <div className="h-5 relative w-full overflow-hidden">
                    <AnimatePresence mode="wait" initial={false}>
                        <motion.p
                            key={step}
                            initial={reduce ? false : { y: 8, opacity: 0 }}
                            animate={{ y: 0, opacity: 1 }}
                            exit={reduce ? undefined : { y: -8, opacity: 0 }}
                            transition={{ duration: 0.2 }}
                            className="text-[12.5px] font-medium ai-shimmer-text"
                        >
                            {steps[step]}
                        </motion.p>
                    </AnimatePresence>
                </div>
                {steps.length > 1 && (
                    <div className="flex items-center gap-1" aria-hidden>
                        {steps.map((_, i) => (
                            <span
                                key={i}
                                className={cn(
                                    "h-1 rounded-full transition-all duration-300",
                                    i < step ? "w-3 bg-sky-400" : i === step ? "w-5 bg-sky-600" : "w-3 bg-slate-200",
                                )}
                            />
                        ))}
                    </div>
                )}
            </div>
            {variant === "table" ? <SkeletonRows rows={rows} /> : <SkeletonCards count={rows} className="p-3" />}
        </div>
    );
}

/** Rows shaped like the mailbox picker: checkbox, name and address, a badge. */
export function SkeletonRows({ rows = 6 }: { rows?: number }) {
    return (
        <div aria-hidden>
            {Array.from({ length: rows }, (_, i) => (
                <motion.div
                    key={i}
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 - i * 0.12 }}
                    transition={{ delay: i * 0.05, duration: 0.25 }}
                    className="px-3 py-2.5 flex items-center gap-3 border-b border-slate-100 last:border-b-0"
                >
                    <Bar className="size-3.5 rounded-[4px]" />
                    <Bar className="size-6 rounded-full" />
                    <div className="flex-1 min-w-0 space-y-1.5">
                        <Bar className="h-2.5" />
                        <Bar className="h-2 w-2/5 opacity-70" />
                    </div>
                    <Bar className="hidden sm:block h-4 w-16 rounded" />
                </motion.div>
            ))}
        </div>
    );
}

/** Cards shaped like an account or grant list entry. */
export function SkeletonCards({ count = 3, className }: { count?: number; className?: string }) {
    return (
        <div className={cn("space-y-2", className)} aria-hidden>
            {Array.from({ length: count }, (_, i) => (
                <motion.div
                    key={i}
                    initial={{ opacity: 0, y: 4 }}
                    animate={{ opacity: 1 - i * 0.15, y: 0 }}
                    transition={{ delay: i * 0.06, duration: 0.25 }}
                    className="rounded-md border border-slate-200 px-3 py-3 flex items-center gap-3"
                >
                    <Bar className="size-8 rounded-lg" />
                    <div className="flex-1 min-w-0 space-y-1.5">
                        <Bar className="h-2.5 w-1/2" />
                        <Bar className="h-2 w-1/3 opacity-70" />
                    </div>
                    <Bar className="h-5 w-14 rounded" />
                </motion.div>
            ))}
        </div>
    );
}

/** A thin sweeping bar across the top edge of a list that is refreshing in place. */
export function RefreshBar({ active }: { active: boolean }) {
    return (
        <AnimatePresence>
            {active && (
                <motion.div
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    className="absolute inset-x-0 top-0 h-0.5 overflow-hidden bg-sky-100 z-10"
                    aria-hidden
                >
                    <span className="block h-full w-2/5 bg-sky-500 progress-sweep" />
                </motion.div>
            )}
        </AnimatePresence>
    );
}

/** A file being read: a scan line over the file icon and the step in progress. */
export function FileAnalyzing({ name, icon, steps }: { name: string; icon: React.ReactNode; steps: string[] }) {
    const reduce = useReducedMotion();
    const step = useStep(steps.length, 1400);
    return (
        <div className="flex flex-col items-center" role="status" aria-live="polite">
            <div className="relative size-12 rounded-lg bg-white border border-slate-200 shadow-sm flex items-center justify-center overflow-hidden">
                {icon}
                {!reduce && (
                    <motion.span
                        className="absolute inset-x-0 h-4 bg-gradient-to-b from-transparent via-sky-300/40 to-transparent"
                        animate={{ top: ["-30%", "100%"] }}
                        transition={{ duration: 1.2, repeat: Infinity, ease: "easeInOut" }}
                    />
                )}
            </div>
            <p className="text-[13px] font-medium text-slate-900 mt-2.5 max-w-full truncate">{name}</p>
            <div className="h-5 relative w-full overflow-hidden mt-0.5">
                <AnimatePresence mode="wait" initial={false}>
                    <motion.p
                        key={step}
                        initial={reduce ? false : { y: 8, opacity: 0 }}
                        animate={{ y: 0, opacity: 1 }}
                        exit={reduce ? undefined : { y: -8, opacity: 0 }}
                        transition={{ duration: 0.2 }}
                        className="text-[11.5px] ai-shimmer-text"
                    >
                        {steps[step]}
                    </motion.p>
                </AnimatePresence>
            </div>
        </div>
    );
}

/** One line of status text in the shimmer, for a small inline wait. */
export function InlineWorking({ children }: { children: React.ReactNode }) {
    return (
        <span className="inline-flex items-center gap-1.5" role="status">
            <span className="relative flex size-2">
                <span className="absolute inset-0 rounded-full bg-sky-400 opacity-60 animate-ping" />
                <span className="relative size-2 rounded-full bg-sky-500" />
            </span>
            <span className="ai-shimmer-text">{children}</span>
        </span>
    );
}
