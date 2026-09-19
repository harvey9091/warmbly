// One-time code entry for the 2FA dialogs: six slots for an authenticator
// code, an optional switch to a recovery code, and an inline error that shakes
// the field and clears it. Auto-submits a complete authenticator code unless
// the caller wants an explicit button (disabling 2FA should not fire on the
// sixth digit).

import React from "react";
import { motion } from "framer-motion";
import { Loader2Icon } from "lucide-react";
import { InputOTP, InputOTPGroup, InputOTPSlot } from "@/components/ui/input-otp";
import { cn } from "@/lib/utils";

export default function CodeEntry({
    onSubmit,
    pending,
    error,
    allowRecovery = false,
    autoSubmit = true,
    autoFocus = true,
    onChange,
}: {
    onSubmit: (code: string) => void;
    pending: boolean;
    /** Message to show under the field. Changing it clears the field. */
    error: string | null;
    allowRecovery?: boolean;
    autoSubmit?: boolean;
    autoFocus?: boolean;
    /** Reports the current (trimmed) value so a parent can enable its button. */
    onChange?: (code: string) => void;
}) {
    const [otp, setOtp] = React.useState("");
    const [recovery, setRecovery] = React.useState("");
    const [useRecovery, setUseRecovery] = React.useState(false);
    const [shake, setShake] = React.useState(0);

    React.useEffect(() => {
        if (!error) return;
        setOtp("");
        setRecovery("");
        onChange?.("");
        setShake((n) => n + 1);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [error]);

    const submitRecovery = (e: React.FormEvent) => {
        e.preventDefault();
        const v = recovery.trim();
        if (v && !pending) onSubmit(v);
    };

    return (
        <div className="space-y-3">
            <motion.div
                // Keyed on the error count: the remount replays the shake and
                // re-focuses the field through autoFocus.
                key={shake}
                animate={shake ? { x: [0, -6, 6, -4, 4, 0] } : undefined}
                transition={{ duration: 0.32 }}
            >
                {useRecovery ? (
                    <form onSubmit={submitRecovery}>
                        <input
                            autoFocus={autoFocus}
                            value={recovery}
                            onChange={(e) => {
                                setRecovery(e.target.value);
                                onChange?.(e.target.value.trim());
                            }}
                            placeholder="xxxxx-xxxxx"
                            autoComplete="off"
                            spellCheck={false}
                            aria-label="Recovery code"
                            aria-invalid={!!error}
                            data-ph-mask=""
                            className={cn(
                                "w-full h-12 px-3 rounded-lg border text-center font-mono tracking-[0.18em] text-[15px] text-slate-900 outline-none transition-colors",
                                error
                                    ? "border-rose-300 focus:border-rose-400 focus:ring-2 focus:ring-rose-100"
                                    : "border-slate-200 focus:border-sky-400 focus:ring-2 focus:ring-sky-100",
                            )}
                        />
                    </form>
                ) : (
                    <div className="flex justify-center">
                        <InputOTP
                            maxLength={6}
                            value={otp}
                            autoFocus={autoFocus}
                            disabled={pending}
                            aria-label="Authenticator code"
                            onChange={(v) => {
                                setOtp(v);
                                onChange?.(v);
                                if (autoSubmit && v.length === 6 && !pending) onSubmit(v);
                            }}
                            onKeyDown={(e) => {
                                if (e.key === "Enter" && otp.length === 6 && !pending) onSubmit(otp);
                            }}
                            containerClassName="gap-2"
                        >
                            <InputOTPGroup className="gap-2">
                                {[0, 1, 2, 3, 4, 5].map((i) => (
                                    <InputOTPSlot
                                        key={i}
                                        index={i}
                                        aria-invalid={!!error}
                                        className={cn(
                                            "!w-10 !h-12 !rounded-lg text-base font-semibold !border !shadow-none first:!rounded-lg last:!rounded-lg data-[active=true]:!ring-sky-100 data-[active=true]:!border-sky-400",
                                            error ? "!border-rose-300" : "!border-slate-200",
                                        )}
                                    />
                                ))}
                            </InputOTPGroup>
                        </InputOTP>
                    </div>
                )}
            </motion.div>

            <div className="min-h-[18px] text-center">
                {pending ? (
                    <span className="inline-flex items-center gap-1.5 text-[12px] text-slate-400">
                        <Loader2Icon className="w-3.5 h-3.5 animate-spin" /> Checking…
                    </span>
                ) : error ? (
                    <span role="alert" className="text-[12px] text-rose-600">
                        {error}
                    </span>
                ) : null}
            </div>

            {allowRecovery && (
                <div className="text-center">
                    <button
                        type="button"
                        onClick={() => {
                            setUseRecovery((v) => !v);
                            setOtp("");
                            setRecovery("");
                            onChange?.("");
                        }}
                        className="text-[12px] text-sky-600 hover:text-sky-700 font-medium transition-colors"
                    >
                        {useRecovery ? "Use your authenticator app instead" : "Use a recovery code instead"}
                    </button>
                </div>
            )}
        </div>
    );
}
