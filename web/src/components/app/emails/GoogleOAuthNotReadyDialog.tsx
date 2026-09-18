// GoogleOAuthNotReadyDialog — why the Gmail OAuth row carries a red badge, and
// what to do instead.
//
// Google sign-in is still going through verification for the Gmail scopes
// Warmbly needs, so a consent on it either refuses outright or ends up with a
// mailbox that cannot send. Rather than let someone find that out at send
// time, the picker marks it and this explains the SMTP / IMAP route with the
// app-password steps, which is the path that works today.
//
// Layered above the connect modal: portalled to the body but still inside the
// modal's React tree, so it stops mousedown and click, and marks itself
// data-floating so the modal's own Escape leaves it alone.

import React from "react";
import { createPortal } from "react-dom";
import { AnimatePresence, motion } from "framer-motion";
import {
    AlertTriangleIcon,
    ArrowRightIcon,
    ExternalLinkIcon,
    KeyRoundIcon,
    XIcon,
} from "lucide-react";

const EASE = [0.22, 1, 0.36, 1] as const;

/** Gmail's own server settings, which never change and are the same for Workspace. */
const GMAIL_SETTINGS: Array<{ label: string; host: string; port: string; security: string }> = [
    { label: "SMTP", host: "smtp.gmail.com", port: "465", security: "SSL / TLS" },
    { label: "IMAP", host: "imap.gmail.com", port: "993", security: "SSL / TLS" },
];

export default function GoogleOAuthNotReadyDialog({
    open,
    onClose,
    onUseSmtp,
}: {
    open: boolean;
    onClose: () => void;
    /** Switch the connect modal to the SMTP / IMAP form. */
    onUseSmtp: () => void;
}) {
    const cardRef = React.useRef<HTMLDivElement>(null);

    React.useEffect(() => {
        if (!open) return;
        const previous = document.activeElement as HTMLElement | null;
        cardRef.current?.focus();
        const onKey = (e: KeyboardEvent) => {
            if (e.key !== "Escape") return;
            if (document.querySelector("[role='alertdialog']")) return;
            e.stopPropagation();
            onClose();
        };
        document.addEventListener("keydown", onKey, true);
        return () => {
            document.removeEventListener("keydown", onKey, true);
            previous?.focus?.();
        };
    }, [open, onClose]);

    return createPortal(
        <AnimatePresence>
            {open && (
                <motion.div
                    key="gmail-oauth-overlay"
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    exit={{ opacity: 0 }}
                    transition={{ duration: 0.15 }}
                    onMouseDown={(e) => {
                        e.stopPropagation();
                        onClose();
                    }}
                    onClick={(e) => e.stopPropagation()}
                    className="fixed inset-0 z-[140] flex items-center justify-center bg-slate-900/40 backdrop-blur-[2px] px-4"
                >
                    <motion.div
                        key="gmail-oauth-card"
                        ref={cardRef}
                        tabIndex={-1}
                        role="dialog"
                        aria-modal="true"
                        aria-label="Google sign-in is not ready"
                        data-floating
                        data-nested-modal
                        initial={{ y: 10, opacity: 0, scale: 0.98 }}
                        animate={{ y: 0, opacity: 1, scale: 1 }}
                        exit={{ y: 10, opacity: 0, scale: 0.98 }}
                        transition={{ duration: 0.2, ease: EASE }}
                        onMouseDown={(e) => e.stopPropagation()}
                        className="w-full max-w-[540px] rounded-lg bg-white border border-slate-200 shadow-[0_24px_48px_-12px_rgba(15,23,42,0.22),0_8px_16px_-8px_rgba(15,23,42,0.12)] overflow-hidden flex flex-col max-h-[88dvh] outline-none"
                    >
                        <div className="h-12 px-4 border-b border-slate-200 flex items-center gap-2.5 shrink-0">
                            <AlertTriangleIcon className="w-3.5 h-3.5 text-rose-500" />
                            <span className="text-[10px] uppercase tracking-[0.14em] text-slate-400 font-medium">
                                Google sign-in
                            </span>
                            <button
                                type="button"
                                onClick={onClose}
                                aria-label="Close"
                                className="ml-auto size-7 rounded-md text-slate-500 hover:text-slate-900 hover:bg-slate-100 inline-flex items-center justify-center transition-colors"
                            >
                                <XIcon className="w-3.5 h-3.5" />
                            </button>
                        </div>

                        <div className="flex-1 min-h-0 overflow-y-auto">
                            <div className="p-4 space-y-4">
                                <div className="rounded-md border border-rose-200 bg-rose-50 p-3">
                                    <p className="text-[13px] font-medium text-rose-900">
                                        Connecting Gmail with Google sign-in is not recommended yet
                                    </p>
                                    <p className="text-[12.5px] text-rose-800/90 mt-1.5">
                                        Our Google app is still going through review for the Gmail access
                                        Warmbly needs. Until that is finished, a mailbox connected this way can
                                        fail to send or lose its authorization without warning. Connect it over
                                        SMTP and IMAP instead: it is the same mailbox, it sends today, and
                                        nothing has to change once Google sign-in is ready.
                                    </p>
                                </div>

                                <div>
                                    <div className="text-[10px] uppercase tracking-[0.14em] text-slate-400 font-medium mb-2">
                                        Connect Gmail with an app password
                                    </div>
                                    <ol className="space-y-2">
                                        <Step n={1}>
                                            Turn on 2-Step Verification on the Google account, under{" "}
                                            <ExtLink href="https://myaccount.google.com/security">
                                                Google Account, Security
                                            </ExtLink>
                                            . Google only offers app passwords once it is on.
                                        </Step>
                                        <Step n={2}>
                                            Create an app password at{" "}
                                            <ExtLink href="https://myaccount.google.com/apppasswords">
                                                myaccount.google.com/apppasswords
                                            </ExtLink>
                                            , name it Warmbly, and copy the 16 characters it shows. That is the
                                            only time it is shown.
                                        </Step>
                                        <Step n={3}>
                                            Turn IMAP on in Gmail: Settings, See all settings, Forwarding and
                                            POP/IMAP, Enable IMAP. On Google Workspace an admin may have to
                                            allow IMAP and app passwords for the organization first.
                                        </Step>
                                        <Step n={4}>
                                            Come back here, choose Other (SMTP / IMAP), and use the app password
                                            as the password with the settings below.
                                        </Step>
                                    </ol>
                                </div>

                                <div className="rounded-md border border-slate-200 overflow-hidden">
                                    <div className="px-3 h-8 flex items-center gap-1.5 border-b border-slate-200 bg-slate-50">
                                        <KeyRoundIcon className="w-3 h-3 text-slate-500" />
                                        <span className="text-[10px] uppercase tracking-[0.14em] text-slate-400 font-medium">
                                            Gmail server settings
                                        </span>
                                    </div>
                                    {GMAIL_SETTINGS.map((s) => (
                                        <div
                                            key={s.label}
                                            className="px-3 py-2 flex items-center gap-3 text-[12px] border-b border-slate-100 last:border-b-0"
                                        >
                                            <span className="w-10 shrink-0 text-slate-500 font-medium">{s.label}</span>
                                            <span className="font-mono text-slate-900 truncate">{s.host}</span>
                                            <span className="font-mono tabular-nums text-slate-600">:{s.port}</span>
                                            <span className="ml-auto text-slate-500 shrink-0">{s.security}</span>
                                        </div>
                                    ))}
                                    <div className="px-3 py-2 text-[11.5px] text-slate-500 border-t border-slate-100">
                                        Username is the full address, including{" "}
                                        <code className="bg-slate-100 px-1 rounded">@gmail.com</code> or your
                                        Workspace domain. Password is the app password, not the account
                                        password.
                                    </div>
                                </div>
                            </div>
                        </div>

                        <div className="px-4 py-3 border-t border-slate-200 flex items-center gap-2 shrink-0">
                            <a
                                href="https://docs.warmbly.com/guides/mailboxes/"
                                target="_blank"
                                rel="noreferrer"
                                className="h-7 px-2.5 inline-flex items-center gap-1.5 rounded-md border border-slate-200 text-[12.5px] text-slate-700 hover:bg-slate-50 transition-colors"
                            >
                                <ExternalLinkIcon className="w-3.5 h-3.5" />
                                Mailbox guide
                            </a>
                            <button
                                type="button"
                                onClick={onUseSmtp}
                                className="ml-auto h-7 px-3 rounded-md bg-slate-900 hover:bg-slate-800 text-white text-[12.5px] font-medium inline-flex items-center gap-1.5 transition-colors"
                            >
                                Use SMTP / IMAP
                                <ArrowRightIcon className="w-3.5 h-3.5" />
                            </button>
                        </div>
                    </motion.div>
                </motion.div>
            )}
        </AnimatePresence>,
        document.body,
    );
}

function Step({ n, children }: { n: number; children: React.ReactNode }) {
    return (
        <li className="flex items-start gap-2.5">
            <span className="size-4 mt-0.5 shrink-0 rounded-full bg-slate-100 text-slate-600 text-[10px] font-medium inline-flex items-center justify-center tabular-nums">
                {n}
            </span>
            <span className="text-[12.5px] text-slate-700 leading-[1.5]">{children}</span>
        </li>
    );
}

function ExtLink({ href, children }: { href: string; children: React.ReactNode }) {
    return (
        <a
            href={href}
            target="_blank"
            rel="noreferrer"
            className="text-sky-700 underline decoration-sky-300 hover:decoration-sky-600 transition-colors"
        >
            {children}
        </a>
    );
}
