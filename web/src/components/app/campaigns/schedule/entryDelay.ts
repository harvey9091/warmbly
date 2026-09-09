// Vocabulary for the campaign's entry delay — how long a contact's FIRST email
// waits after they entered the campaign. Kept apart from the picker component so
// the Schedule tab, the flow canvas and any future surface share one wording.

// Ceiling mirrors validate.CampaignEntryDelayMaxMinutes and the column's CHECK.
export const ENTRY_DELAY_MAX_MINUTES = 90 * 24 * 60;

export const ENTRY_DELAY_PRESETS: { label: string; minutes: number }[] = [
    { label: "Immediately", minutes: 0 },
    { label: "1 hour", minutes: 60 },
    { label: "4 hours", minutes: 240 },
    { label: "1 day", minutes: 1440 },
    { label: "2 days", minutes: 2880 },
    { label: "3 days", minutes: 4320 },
    { label: "1 week", minutes: 10080 },
];

export const ENTRY_DELAY_UNIT_MINUTES = { minutes: 1, hours: 60, days: 1440 } as const;
export type EntryDelayUnit = keyof typeof ENTRY_DELAY_UNIT_MINUTES;

// Largest whole unit the value divides into, so 2880 reads as "2 days".
export function splitEntryDelay(minutes: number): { amount: number; unit: EntryDelayUnit } {
    if (minutes > 0 && minutes % ENTRY_DELAY_UNIT_MINUTES.days === 0) {
        return { amount: minutes / ENTRY_DELAY_UNIT_MINUTES.days, unit: "days" };
    }
    if (minutes > 0 && minutes % ENTRY_DELAY_UNIT_MINUTES.hours === 0) {
        return { amount: minutes / ENTRY_DELAY_UNIT_MINUTES.hours, unit: "hours" };
    }
    return { amount: minutes, unit: "minutes" };
}

/** "Immediately", "2 days", "90 minutes" — the phrase every surface shows. */
export function entryDelayLabel(minutes: number): string {
    if (minutes <= 0) return "Immediately";
    const preset = ENTRY_DELAY_PRESETS.find((p) => p.minutes === minutes);
    if (preset) return preset.label;
    const { amount, unit } = splitEntryDelay(minutes);
    return `${amount} ${amount === 1 ? unit.slice(0, -1) : unit}`;
}
