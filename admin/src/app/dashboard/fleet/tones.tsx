// Badge tones shared by the fleet tabs, derived from the legends so the
// pills here read the same as everywhere else.

import { Badge } from "@/components/ui/badge";
import { WORKER_HEALTH_LEGEND } from "@/lib/legends";
import { cn } from "@/lib/utils";

const HEALTH_TONE = Object.fromEntries(WORKER_HEALTH_LEGEND.map((e) => [e.term, e.tone ?? ""]));
const FALLBACK = "border-zinc-300 text-zinc-600";

export function HealthPill({ state }: { state: string }) {
    return (
        <Badge variant="outline" className={cn("text-[10px]", HEALTH_TONE[state] ?? FALLBACK)}>
            {state || "unknown"}
        </Badge>
    );
}

export function LiveDot({ live, title }: { live: boolean; title?: string }) {
    return (
        <span className="inline-flex items-center gap-1.5 text-xs" title={title}>
            <span
                className={cn(
                    "inline-block size-2 rounded-full",
                    live ? "bg-emerald-500" : "bg-zinc-300",
                )}
            />
            <span className={live ? "text-emerald-700" : "text-muted-foreground"}>
                {live ? "live" : "offline"}
            </span>
        </span>
    );
}
