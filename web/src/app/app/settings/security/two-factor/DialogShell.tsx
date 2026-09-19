// Modal shell shared by the 2FA dialogs: backdrop, animated card, header with
// an optional close, and Escape handling that only closes the innermost layer.
// Pass no `onClose` to lock the dialog (the recovery-codes step must not be
// dismissed until the user confirms the codes are saved).

import React from "react";
import { motion } from "framer-motion";
import { XIcon } from "lucide-react";
import { cn } from "@/lib/utils";

export default function DialogShell({
    title,
    icon,
    onClose,
    width = "sm",
    children,
    footer,
    label,
}: {
    title: React.ReactNode;
    icon?: React.ReactNode;
    onClose?: () => void;
    width?: "sm" | "md";
    children: React.ReactNode;
    footer?: React.ReactNode;
    label?: string;
}) {
    const cardRef = React.useRef<HTMLDivElement>(null);

    // Move focus in on open (unless a field already took it), keep Tab inside
    // the card, and hand focus back to whatever opened the dialog on close.
    React.useEffect(() => {
        const opener = document.activeElement as HTMLElement | null;
        const card = cardRef.current;
        if (card && !card.contains(document.activeElement)) card.focus();
        const onTab = (e: KeyboardEvent) => {
            if (e.key !== "Tab" || !cardRef.current) return;
            const items = Array.from(
                cardRef.current.querySelectorAll<HTMLElement>(
                    'button:not([disabled]), input:not([disabled]), a[href], [tabindex]:not([tabindex="-1"])',
                ),
            );
            if (items.length === 0) return;
            const first = items[0];
            const last = items[items.length - 1];
            const active = document.activeElement;
            if (e.shiftKey && (active === first || !cardRef.current.contains(active))) {
                e.preventDefault();
                last.focus();
            } else if (!e.shiftKey && (active === last || !cardRef.current.contains(active))) {
                e.preventDefault();
                first.focus();
            }
        };
        document.addEventListener("keydown", onTab);
        return () => {
            document.removeEventListener("keydown", onTab);
            if (opener && document.contains(opener)) opener.focus();
        };
    }, []);

    React.useEffect(() => {
        if (!onClose) return;
        const onKey = (e: KeyboardEvent) => {
            if (e.key !== "Escape") return;
            // A popover or the confirm dialog on top of us owns Escape.
            if (document.querySelector('[data-floating], [role="alertdialog"]')) return;
            e.stopPropagation();
            onClose();
        };
        document.addEventListener("keydown", onKey);
        return () => document.removeEventListener("keydown", onKey);
    }, [onClose]);

    return (
        <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.14 }}
            className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/30 p-4"
            onMouseDown={() => onClose?.()}
        >
            <motion.div
                ref={cardRef}
                tabIndex={-1}
                role="dialog"
                aria-modal="true"
                aria-label={label ?? (typeof title === "string" ? title : undefined)}
                initial={{ y: 8, opacity: 0, scale: 0.985 }}
                animate={{ y: 0, opacity: 1, scale: 1 }}
                exit={{ y: 8, opacity: 0, scale: 0.985 }}
                transition={{ duration: 0.18, ease: [0.22, 1, 0.36, 1] }}
                onMouseDown={(e) => e.stopPropagation()}
                className={cn(
                    "w-full max-h-[calc(100dvh-2rem)] flex flex-col rounded-lg outline-none bg-white border border-slate-200 overflow-hidden",
                    "shadow-[0_24px_48px_-12px_rgba(15,23,42,0.18),0_8px_16px_-8px_rgba(15,23,42,0.1)]",
                    width === "md" ? "max-w-[560px]" : "max-w-[400px]",
                )}
            >
                <div className="h-12 px-4 border-b border-slate-200 flex items-center gap-2.5 shrink-0">
                    {icon && (
                        <div className="size-5 rounded bg-slate-100 text-slate-600 flex items-center justify-center">
                            {icon}
                        </div>
                    )}
                    <span className="text-[13px] font-medium text-slate-900 truncate">{title}</span>
                    {onClose && (
                        <button
                            type="button"
                            onClick={onClose}
                            aria-label="Close"
                            className="ml-auto h-7 w-7 rounded-md inline-flex items-center justify-center text-slate-400 hover:text-slate-700 hover:bg-slate-100 transition-colors"
                        >
                            <XIcon className="w-4 h-4" />
                        </button>
                    )}
                </div>
                <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden">{children}</div>
                {footer && (
                    <div className="px-4 min-h-[52px] border-t border-slate-200 bg-slate-50/40 flex items-center gap-2 shrink-0">
                        {footer}
                    </div>
                )}
            </motion.div>
        </motion.div>
    );
}

export function PrimaryButton({
    children,
    className,
    ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement>) {
    return (
        <button
            type="button"
            {...props}
            className={cn(
                "h-8 px-3 rounded-md bg-sky-600 text-white text-[12.5px] font-medium hover:bg-sky-700 transition-colors inline-flex items-center justify-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed",
                className,
            )}
        >
            {children}
        </button>
    );
}

export function SecondaryButton({
    children,
    className,
    ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement>) {
    return (
        <button
            type="button"
            {...props}
            className={cn(
                "h-8 px-3 rounded-md border border-slate-200 text-[12.5px] text-slate-700 hover:border-slate-300 hover:text-slate-900 transition-colors inline-flex items-center justify-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed",
                className,
            )}
        >
            {children}
        </button>
    );
}
