// Snooze times, shared by the thread header and the conversation row's menu.
// One definition so the two offer the same presets and the same ceiling.

/** Server ceiling, matching SnoozeMaxHorizon in internal/app/unibox/config.go. */
export const SNOOZE_MAX_MS = 90 * 24 * 60 * 60 * 1000;

export function offsetHours(h: number): Date {
    const d = new Date();
    d.setHours(d.getHours() + h);
    return d;
}

export function offsetDays(days: number): Date {
    const d = new Date();
    d.setDate(d.getDate() + days);
    return d;
}

export function atHour(dayOffset: number, hour: number): Date {
    const d = new Date();
    d.setDate(d.getDate() + dayOffset);
    d.setHours(hour, 0, 0, 0);
    return d;
}

export function nextMonday9(): Date {
    const d = new Date();
    const dow = d.getDay();
    const delta = (1 - dow + 7) % 7 || 7;
    d.setDate(d.getDate() + delta);
    d.setHours(9, 0, 0, 0);
    return d;
}

export const SNOOZE_PRESETS: { label: string; until: () => Date }[] = [
    { label: "In 1 hour", until: () => offsetHours(1) },
    { label: "In 3 hours", until: () => offsetHours(3) },
    { label: "Tomorrow 9:00", until: () => atHour(1, 9) },
    { label: "Monday 9:00", until: () => nextMonday9() },
    { label: "Next week", until: () => offsetDays(7) },
];
