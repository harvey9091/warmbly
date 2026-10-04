// The standalone dialog an import view opens in outside the connect modal: the
// Imports menu reopening a job, and the mailbox drawer opening a grant.
// Escape, the backdrop and the close button share one guarded close.
import React from "react";
import { createPortal } from "react-dom";
import { AnimatePresence, motion } from "framer-motion";
import { XIcon } from "lucide-react";
import MailboxAllowanceDialog from "@/components/app/emails/MailboxAllowanceDialog";
import useMailboxAllowance from "@/lib/api/hooks/app/emails/useMailboxAllowance";
import { allowanceFull } from "@/lib/api/models/app/emails/MailboxAllowance";
import useFeatureAccess from "@/hooks/useFeatureAccess";
import { useConfirm } from "@/hooks/context/confirm";
import { useMailboxSourceBusy } from "@/lib/api/hooks/app/emails/mailboxSourceBusy";

export interface ImportDialogContext {
    onAllowance: () => void;
    setDirty: (dirty: boolean) => void;
}

export default function ImportDialogShell({
    open,
    title,
    icon,
    onClose,
    discardText = "Discard this import? The list and the choices made for it are lost.",
    children,
}: {
    open: boolean;
    title: string;
    icon: React.ReactNode;
    onClose: () => void;
    discardText?: string;
    children: (ctx: ImportDialogContext) => React.ReactNode;
}) {
    const allowance = useMailboxAllowance(open);
    const access = useFeatureAccess();
    const [allowanceOpen, setAllowanceOpen] = React.useState(false);
    const [dirty, setDirty] = React.useState(false);

    React.useEffect(() => {
        if (!open) {
            setAllowanceOpen(false);
            setDirty(false);
        }
    }, [open]);

    const confirm = useConfirm();
    // A request in flight (an import starting, a grant being recorded) finishes before the dialog can go.
    const busy = useMailboxSourceBusy() && open;
    const requestClose = React.useCallback(() => {
        if (busy) return;
        if (dirty) {
            confirm.show(discardText, async () => onClose());
            return;
        }
        onClose();
    }, [busy, dirty, confirm, onClose, discardText]);

    React.useEffect(() => {
        if (!open) return;
        const onKey = (e: KeyboardEvent) => {
            if (e.key !== "Escape") return;
            if (document.querySelector("[data-floating], [role='alertdialog']") || allowanceOpen) return;
            e.preventDefault();
            requestClose();
        };
        document.addEventListener("keydown", onKey);
        return () => document.removeEventListener("keydown", onKey);
    }, [open, requestClose, allowanceOpen]);

    const ctx = React.useMemo<ImportDialogContext>(() => ({ onAllowance: () => setAllowanceOpen(true), setDirty }), []);

    // Portaled: an opener inside a sliding drawer has a transform that would trap a fixed overlay.
    return createPortal(
        <AnimatePresence>
            {open && (
                <motion.div
                    key="overlay"
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    transition={{ duration: 0.15 }}
                    onMouseDown={requestClose}
                    className="fixed inset-0 z-[110] flex items-center justify-center bg-slate-900/30 backdrop-blur-[2px] px-4"
                >
                    <motion.div
                        key="card"
                        role="dialog"
                        aria-modal="true"
                        aria-label={title}
                        initial={{ y: 8, opacity: 0 }}
                        animate={{ y: 0, opacity: 1 }}
                        exit={{ y: 8, opacity: 0 }}
                        transition={{ duration: 0.16 }}
                        onMouseDown={(e) => e.stopPropagation()}
                        className="w-full max-w-[760px] rounded-lg bg-white border border-slate-200 shadow-[0_24px_48px_-12px_rgba(15,23,42,0.18),0_8px_16px_-8px_rgba(15,23,42,0.1)] overflow-hidden flex flex-col max-h-[88dvh]"
                    >
                        <div className="h-12 px-4 border-b border-slate-200 flex items-center gap-2.5 shrink-0">
                            <div className="size-5 rounded bg-slate-100 text-slate-600 flex items-center justify-center">{icon}</div>
                            <span className="text-[10px] uppercase tracking-[0.14em] text-slate-400 font-medium">Mailbox</span>
                            <div className="h-4 w-px bg-slate-200" />
                            <span className="text-[12px] text-slate-600 truncate">{title}</span>
                            <button
                                type="button"
                                onClick={requestClose}
                                disabled={busy}
                                aria-label="Close"
                                title={busy ? "Wait for this to finish" : undefined}
                                className="ml-auto size-7 rounded-md text-slate-500 hover:text-slate-900 hover:bg-slate-100 inline-flex items-center justify-center transition-colors disabled:opacity-40 disabled:hover:bg-transparent disabled:cursor-not-allowed"
                            >
                                <XIcon className="w-3.5 h-3.5" />
                            </button>
                        </div>
                        <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden">{children(ctx)}</div>
                    </motion.div>
                    <div onMouseDown={(e) => e.stopPropagation()}>
                        <MailboxAllowanceDialog
                            open={allowanceOpen}
                            onClose={() => setAllowanceOpen(false)}
                            allowance={allowance.data}
                            reached={allowanceFull(allowance.data)}
                            currentPlan={access.plan}
                        />
                    </div>
                </motion.div>
            )}
        </AnimatePresence>,
        document.body,
    );
}
