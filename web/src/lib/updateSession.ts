// Remembers that this browser started an instance update, across the backend
// restart and a reload, so the header can report the outcome once the backend
// answers again instead of going quiet.

const KEY = "warmbly.update.started";

export interface StartedUpdate {
    fromVersion: string;
    fromCommit: string;
    startedAt: number;
}

export function markUpdateStarted(fromVersion: string, fromCommit: string) {
    try {
        sessionStorage.setItem(KEY, JSON.stringify({ fromVersion, fromCommit, startedAt: Date.now() }));
    } catch {
        /* no storage: the open dialog still tracks the job */
    }
}

export function readUpdateStarted(): StartedUpdate | null {
    try {
        const raw = sessionStorage.getItem(KEY);
        if (!raw) return null;
        const v = JSON.parse(raw) as StartedUpdate;
        if (Date.now() - v.startedAt > 60 * 60_000) {
            sessionStorage.removeItem(KEY);
            return null;
        }
        return v;
    } catch {
        return null;
    }
}

export function clearUpdateStarted() {
    try {
        sessionStorage.removeItem(KEY);
    } catch {
        /* ignore */
    }
}
