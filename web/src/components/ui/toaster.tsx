import { Toaster as HotToaster, ToastBar, toast } from "react-hot-toast";
import { motion } from "motion/react";
import { XIcon } from "lucide-react";

// A server's verdict on a mailbox can run to several sentences. react-hot-toast
// caps a toast at 350px, which turns one of those into a narrow column that
// covers the form and is gone before it is read, so errors get a wide card, a
// longer stay and a close button.
const TOAST_MAX_WIDTH = 640;
const ERROR_DURATION = 12000;

export function Toaster() {
    return (
        <HotToaster
            position="top-center"
            toastOptions={{
                duration: 4000,
                style: { maxWidth: TOAST_MAX_WIDTH },
                error: { duration: ERROR_DURATION },
            }}
        >
            {(t) => (
                <motion.div
                    initial={{ opacity: 0, y: -8, scale: 0.96 }}
                    animate={{
                        opacity: t.visible ? 1 : 0,
                        y: t.visible ? 0 : -8,
                        scale: t.visible ? 1 : 0.96,
                    }}
                    transition={{ duration: 0.25, ease: "easeOut" }}
                >
                    <ToastBar toast={t} style={{ animation: "none", maxWidth: TOAST_MAX_WIDTH }}>
                        {({ icon, message }) => (
                            <>
                                {icon}
                                <div className="min-w-0 flex-1 text-[12.5px] leading-[1.5] break-words">{message}</div>
                                {t.type === "error" && (
                                    <button
                                        type="button"
                                        aria-label="Dismiss"
                                        onClick={() => toast.dismiss(t.id)}
                                        className="shrink-0 self-start -mr-1 size-6 rounded-md inline-flex items-center justify-center text-slate-400 hover:text-slate-700 hover:bg-slate-100 transition-colors"
                                    >
                                        <XIcon className="w-3.5 h-3.5" />
                                    </button>
                                )}
                            </>
                        )}
                    </ToastBar>
                </motion.div>
            )}
        </HotToaster>
    );
}
