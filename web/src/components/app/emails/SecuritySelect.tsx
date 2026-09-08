// Segmented picker for a mailbox leg's connection security. Shared by the
// connect modal and the reconnect dialog so both describe the choice the same
// way. Theme primitives only: h-7 control, slate border, sky active state.
//
// The unencrypted option is not a permanent third segment. It appears only
// once the host is a loopback literal on a self-hosted instance, which is the
// only shape the backend and the worker accept, so the form never offers a
// mode that is going to be refused.

import { cn } from "@/lib/utils";
import { allowsNoEncryption, type MailSecurity } from "@/lib/api/models/app/emails/Service";

const OPTIONS: { value: MailSecurity; label: string; hint: string }[] = [
    { value: "tls", label: "SSL / TLS", hint: "Encrypted from the first byte (SMTP 465, IMAP 993)" },
    { value: "starttls", label: "STARTTLS", hint: "Upgrades after connecting (SMTP 587 or 2525, IMAP 143)" },
    { value: "none", label: "None", hint: "No encryption. Only to a mail server on this machine, such as Proton Bridge" },
];

export default function SecuritySelect({
    value,
    onChange,
    host = "",
    selfHosted = false,
}: {
    value: MailSecurity;
    onChange: (v: MailSecurity) => void;
    /** The leg's host, which decides whether "None" is offered at all. */
    host?: string;
    /** Hosted instances never run the worker on the customer's machine. */
    selfHosted?: boolean;
}) {
    const allowNone = allowsNoEncryption(host, selfHosted);
    const options = OPTIONS.filter((o) => o.value !== "none" || allowNone);

    return (
        <div>
            <div className="flex items-stretch h-7 rounded-md border border-slate-200 bg-white overflow-hidden">
                {options.map((o, i) => (
                    <button
                        key={o.value}
                        type="button"
                        title={o.hint}
                        aria-pressed={value === o.value}
                        onClick={() => onChange(o.value)}
                        className={cn(
                            "flex-1 min-w-0 px-2 text-[12.5px] transition-colors",
                            i > 0 && "border-l border-slate-200",
                            value === o.value
                                ? o.value === "none"
                                    ? "bg-amber-50 text-amber-700 font-medium"
                                    : "bg-sky-50 text-sky-700 font-medium"
                                : "text-slate-600 hover:bg-slate-50",
                        )}
                    >
                        {o.label}
                    </button>
                ))}
            </div>
            {value === "none" && allowNone && (
                <p className="mt-1 text-[11.5px] leading-[1.4] text-slate-500">
                    Credentials go over an unencrypted connection to this machine only. Warmbly refuses this mode for any
                    other host.
                </p>
            )}
        </div>
    );
}
