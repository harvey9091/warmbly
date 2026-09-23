// When a new contact's first email goes out: straight away, or after a wait.
// Edits campaigns.entry_delay_minutes, the same value as the Steps canvas's
// Trigger card.

import React from "react";
import { AnimatePresence, motion } from "framer-motion";
import { HourglassIcon, MailIcon, UserPlusIcon, ZapIcon } from "lucide-react";
import type Campaign from "@/lib/api/models/app/campaigns/Campaign";
import { Label } from "@/components/ui/field";
import EntryDelayPicker from "@/components/app/campaigns/schedule/EntryDelayPicker";
import { entryDelayLabel } from "@/components/app/campaigns/schedule/entryDelay";
import { cn } from "@/lib/utils";
import { OptionSelect } from "./components/CampaignPreferenceBoolBox";

// What "After a wait" starts from when the campaign has never had one.
const DEFAULT_WAIT_MINUTES = 24 * 60;

type Mode = "now" | "wait";

export function FirstEmailSection({
    newCampaign,
    setNewCampaign,
}: {
    newCampaign: Campaign;
    setNewCampaign: React.Dispatch<React.SetStateAction<Campaign>>;
}) {
    const minutes = newCampaign.entry_delay_minutes ?? 0;
    const mode: Mode = minutes > 0 ? "wait" : "now";

    // Switching to "As soon as they join" and back restores the wait you had.
    const [lastWait, setLastWait] = React.useState(minutes > 0 ? minutes : DEFAULT_WAIT_MINUTES);

    const set = (next: number) => {
        if (next > 0) setLastWait(next);
        setNewCampaign((bef) => ({ ...bef, entry_delay_minutes: next }));
    };

    return (
        <div className="space-y-4">
            <OptionSelect<Mode>
                aria-label="When the first email goes out"
                cols={2}
                value={mode}
                onChange={(v) => set(v === "now" ? 0 : minutes || lastWait)}
                options={[
                    { value: "now", label: "As soon as they join", hint: "Goes out in the next sending window." },
                    { value: "wait", label: "After a wait", hint: "Give each new contact some time first." },
                ]}
            />

            <AnimatePresence initial={false}>
                {mode === "wait" && (
                    <motion.div
                        key="wait"
                        initial={{ height: 0, opacity: 0 }}
                        animate={{ height: "auto", opacity: 1 }}
                        exit={{ height: 0, opacity: 0 }}
                        transition={{ duration: 0.18, ease: [0.16, 1, 0.3, 1] }}
                        className="overflow-hidden"
                    >
                        <Label>How long to wait</Label>
                        <div className="mt-1">
                            <EntryDelayPicker withoutImmediate value={minutes} onChange={set} />
                        </div>
                    </motion.div>
                )}
            </AnimatePresence>

            <FirstEmailTimeline minutes={minutes} />

            <p className="text-[11px] leading-relaxed text-slate-400">
                Counted from when each contact joins, so someone added next month waits just as long. Follow-up waits
                are set on each step. This is also the Trigger card at the top of the Steps tab.
            </p>
        </div>
    );
}

// Contact joins → (wait) → first email, drawn so the setting reads at a glance.
function FirstEmailTimeline({ minutes }: { minutes: number }) {
    const waits = minutes > 0;
    return (
        <div className="rounded-md border border-slate-200 bg-slate-50/60 px-3 sm:px-5 py-3.5">
            <div className="flex items-start">
                <TimelineStop
                    icon={<UserPlusIcon className="w-3.5 h-3.5" />}
                    title="Contact joins"
                    hint="Added to this campaign"
                />
                <div className="relative flex-1 min-w-[72px] mt-3.5 mx-1">
                    <div
                        className={cn(
                            "border-t",
                            waits ? "border-dashed border-sky-300" : "border-solid border-slate-300",
                        )}
                    />
                    <span
                        className={cn(
                            "absolute left-1/2 top-0 -translate-x-1/2 -translate-y-1/2 inline-flex items-center gap-1 h-5 px-1.5 rounded-full border text-[10.5px] font-medium whitespace-nowrap",
                            waits
                                ? "border-sky-200 bg-sky-50 text-sky-700"
                                : "border-slate-200 bg-white text-slate-500",
                        )}
                    >
                        {waits ? <HourglassIcon className="w-3 h-3" /> : <ZapIcon className="w-3 h-3" />}
                        {waits ? `waits ${entryDelayLabel(minutes).toLowerCase()}` : "no wait"}
                    </span>
                </div>
                <TimelineStop
                    icon={<MailIcon className="w-3.5 h-3.5" />}
                    title="First email"
                    hint="In the next sending window"
                    accent
                />
            </div>
        </div>
    );
}

function TimelineStop({
    icon,
    title,
    hint,
    accent = false,
}: {
    icon: React.ReactNode;
    title: string;
    hint: string;
    accent?: boolean;
}) {
    return (
        <div className="w-[92px] sm:w-[120px] shrink-0 flex flex-col items-center text-center">
            <span
                className={cn(
                    "size-7 rounded-full inline-flex items-center justify-center ring-1",
                    accent ? "bg-sky-600 text-white ring-sky-600" : "bg-white text-slate-500 ring-slate-200",
                )}
            >
                {icon}
            </span>
            <span className="mt-1.5 text-[11.5px] font-medium text-slate-800 leading-tight">{title}</span>
            <span className="mt-0.5 text-[10.5px] text-slate-400 leading-tight">{hint}</span>
        </div>
    );
}
