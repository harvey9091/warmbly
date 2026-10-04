// The import wizards' shared chrome: the numbered stepper, the directional
// pane slide, and the sticky footer that explains why a step cannot be left.
import React from "react";
import { AnimatePresence, motion } from "framer-motion";
import { AlertCircleIcon, CheckIcon, ChevronLeftIcon } from "lucide-react";
import { cn } from "@/lib/utils";

export interface WizardStep<K extends string> {
    key: K;
    label: string;
}

const paneVariants = {
    enter: (dir: 1 | -1) => ({ x: dir * 28, opacity: 0 }),
    center: { x: 0, opacity: 1 },
    exit: (dir: 1 | -1) => ({ x: dir * -28, opacity: 0 }),
};

export function Stepper<K extends string>({
    steps,
    at,
    canClick,
    goTo,
    titleFor,
}: {
    steps: WizardStep<K>[];
    at: number;
    canClick: (key: K, index: number) => boolean;
    goTo: (key: K) => void;
    titleFor?: (key: K) => string | undefined;
}) {
    return (
        <div className="px-4 h-11 border-b border-slate-100 flex items-center shrink-0 bg-white sticky top-0 z-[2]">
            {steps.map((s, i) => {
                const active = i === at;
                const done = i < at;
                const clickable = !active && canClick(s.key, i);
                return (
                    <React.Fragment key={s.key}>
                        <button
                            type="button"
                            onClick={() => clickable && goTo(s.key)}
                            disabled={!clickable}
                            aria-current={active ? "step" : undefined}
                            title={titleFor?.(s.key)}
                            className={cn(
                                "inline-flex items-center gap-2 h-7 pl-1 pr-2 rounded-md shrink-0 transition-colors outline-none focus-visible:ring-2 focus-visible:ring-sky-100",
                                clickable ? "hover:bg-slate-100" : "cursor-default",
                            )}
                        >
                            <span
                                className={cn(
                                    "size-5 rounded-full inline-flex items-center justify-center text-[10.5px] font-semibold tabular-nums transition-colors",
                                    done
                                        ? "bg-sky-600 text-white"
                                        : active
                                          ? "bg-white text-sky-700 ring-1 ring-inset ring-sky-600"
                                          : "bg-white text-slate-400 ring-1 ring-inset ring-slate-200",
                                )}
                            >
                                {done ? <CheckIcon className="w-3 h-3" strokeWidth={3} /> : i + 1}
                            </span>
                            <span
                                className={cn(
                                    "text-[11.5px] font-medium whitespace-nowrap",
                                    active ? "text-slate-900 inline" : done ? "text-slate-600 hidden sm:inline" : "text-slate-400 hidden sm:inline",
                                )}
                            >
                                {s.label}
                            </span>
                        </button>
                        {i < steps.length - 1 && (
                            <span className="relative flex-1 h-px mx-1 sm:mx-2 bg-slate-200 min-w-3 overflow-hidden">
                                <motion.span
                                    initial={false}
                                    animate={{ scaleX: done ? 1 : 0 }}
                                    transition={{ duration: 0.25, ease: [0.22, 1, 0.36, 1] }}
                                    style={{ originX: 0 }}
                                    className="absolute inset-0 bg-sky-600"
                                />
                            </span>
                        )}
                    </React.Fragment>
                );
            })}
        </div>
    );
}

/** The current step's pane, sliding in from the side it was reached from. */
export function WizardPanes({ stepKey, direction, children }: { stepKey: string; direction: 1 | -1; children: React.ReactNode }) {
    return (
        <div className="overflow-x-hidden">
            <AnimatePresence mode="wait" initial={false} custom={direction}>
                <motion.div
                    key={stepKey}
                    custom={direction}
                    variants={paneVariants}
                    initial="enter"
                    animate="center"
                    exit="exit"
                    transition={{ duration: 0.18, ease: [0.22, 1, 0.36, 1] }}
                    className="min-h-[300px]"
                >
                    {children}
                </motion.div>
            </AnimatePresence>
        </div>
    );
}

/**
 * Back on the left, the step's primary action on the right, and the reason the
 * step cannot be left once someone tries. `floating` sits above the bar,
 * centered, for a selection bar.
 */
export function WizardFooter({
    onBack,
    backDisabled,
    note,
    issue,
    nudged,
    primary,
    floating,
}: {
    onBack?: () => void;
    backDisabled?: boolean;
    note?: React.ReactNode;
    issue: string | null;
    nudged: boolean;
    primary: React.ReactNode;
    floating?: React.ReactNode;
}) {
    return (
        <div className="px-3 min-h-12 py-1.5 sm:py-0 sm:h-12 border-t border-slate-200 flex items-center gap-1.5 bg-slate-50/80 backdrop-blur-sm sticky bottom-0 z-[2]">
            {/* Centered by flex so the motion transform does not fight a translate. */}
            <div className="absolute bottom-full inset-x-0 mb-2 px-2 flex justify-center pointer-events-none">
                <AnimatePresence>
                    {floating && (
                        <motion.div
                            key="floating"
                            initial={{ opacity: 0, y: 8 }}
                            animate={{ opacity: 1, y: 0 }}
                            exit={{ opacity: 0, y: 8 }}
                            transition={{ duration: 0.16, ease: [0.22, 1, 0.36, 1] }}
                            className="pointer-events-auto max-w-full"
                        >
                            {floating}
                        </motion.div>
                    )}
                </AnimatePresence>
            </div>
            {onBack ? (
                <button
                    type="button"
                    onClick={onBack}
                    disabled={backDisabled}
                    className="h-7 px-2.5 rounded-md text-[12px] text-slate-700 hover:text-slate-900 hover:bg-slate-100 inline-flex items-center gap-1 transition-colors disabled:opacity-50"
                >
                    <ChevronLeftIcon className="w-3 h-3" />
                    Back
                </button>
            ) : note ? (
                <span className="text-[11px] text-slate-400 pl-1 hidden sm:inline truncate">{note}</span>
            ) : null}
            <div className="ml-auto flex items-center gap-2.5 min-w-0">
                <AnimatePresence initial={false}>
                    {nudged && issue && (
                        <motion.span
                            key={issue}
                            initial={{ opacity: 0, x: 6 }}
                            animate={{ opacity: 1, x: 0 }}
                            exit={{ opacity: 0, x: 6 }}
                            transition={{ duration: 0.14 }}
                            role="status"
                            className="text-[11.5px] text-amber-700 inline-flex items-center gap-1 min-w-0"
                        >
                            <AlertCircleIcon className="w-3 h-3 shrink-0" />
                            <span className="truncate">{issue}</span>
                        </motion.span>
                    )}
                </AnimatePresence>
                {primary}
            </div>
        </div>
    );
}

/** The footer's slate primary button; stays clickable when blocked so a click can say why. */
export function PrimaryButton({
    blocked,
    pending,
    onClick,
    children,
}: {
    blocked: boolean;
    pending?: boolean;
    onClick: () => void;
    children: React.ReactNode;
}) {
    return (
        <motion.button
            type="button"
            onClick={onClick}
            whileTap={!blocked && !pending ? { scale: 0.97 } : undefined}
            aria-disabled={blocked || pending}
            className={cn(
                "shrink-0 h-7 px-3 rounded-md bg-slate-900 hover:bg-slate-800 text-white text-[12px] font-medium inline-flex items-center gap-1.5 transition-colors",
                (blocked || pending) && "opacity-50",
            )}
        >
            {children}
        </motion.button>
    );
}
