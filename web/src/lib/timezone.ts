import type Timezone from "@/lib/api/models/app/Timezone";
import type { SelectOption } from "@/components/ui/select-menu";

/** The IANA zone this browser reports, or "" when it cannot say. */
export function browserTimezone(): string {
    try {
        return Intl.DateTimeFormat().resolvedOptions().timeZone || "";
    } catch {
        return "";
    }
}

/**
 * The timezone a new schedule starts in: the workspace's when one is set,
 * otherwise the browser's, otherwise UTC. Mirrors the server's default for a
 * campaign created without a timezone, which is the workspace's, else UTC.
 */
export function defaultScheduleTimezone(workspaceTimezone?: string | null): string {
    return workspaceTimezone || browserTimezone() || "UTC";
}

/** Label for the "follow the workspace" choice, naming the zone it resolves to today. */
export function followWorkspaceLabel(workspaceZone?: string | null): string {
    return `Follow the workspace (${workspaceZone || "UTC"})`;
}

/**
 * Picker options: the curated list from the API, plus any zone in `extra`
 * (the current value, the browser's) it does not carry, so a choice is never
 * shown as blank. The server accepts any IANA name, not only the list.
 */
export function timezoneOptions(list: Timezone[] | undefined, ...extra: (string | null | undefined)[]): SelectOption[] {
    const options: SelectOption[] = (list ?? []).map((tz) => ({ value: tz.name, label: tz.display_name }));
    for (const zone of extra) {
        if (!zone || options.some((o) => o.value === zone)) continue;
        options.unshift({ value: zone, label: `${offsetLabel(zone)} ${zone}`.trim() });
    }
    return options;
}

/** "(UTC-04:00)" for a zone right now, in the same shape the API uses. */
function offsetLabel(zone: string): string {
    const minutes = offsetMinutes(zone);
    if (minutes === null) return "";
    const sign = minutes < 0 ? "-" : "+";
    const abs = Math.abs(minutes);
    const hh = String(Math.floor(abs / 60)).padStart(2, "0");
    const mm = String(abs % 60).padStart(2, "0");
    return `(UTC${sign}${hh}:${mm})`;
}

function offsetMinutes(zone: string, at: Date = new Date()): number | null {
    try {
        const parts = new Intl.DateTimeFormat("en-US", {
            timeZone: zone,
            hourCycle: "h23",
            year: "numeric",
            month: "2-digit",
            day: "2-digit",
            hour: "2-digit",
            minute: "2-digit",
            second: "2-digit",
        }).formatToParts(at);
        const get = (type: string) => Number(parts.find((p) => p.type === type)?.value);
        const wall = Date.UTC(get("year"), get("month") - 1, get("day"), get("hour"), get("minute"), get("second"));
        if (Number.isNaN(wall)) return null;
        return Math.round((wall - at.getTime()) / 60000);
    } catch {
        return null;
    }
}
