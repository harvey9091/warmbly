// Remembers that this browser started an update, across the backend restart
// and a page reload, so the top bar can say "Updated to vX" (or that it
// failed) once the backend answers again instead of silently going quiet.

const KEY = "warmbly.admin.update.started";

export interface StartedUpdate {
    fromVersion: string;
    fromCommit: string;
    startedAt: number;
}

export function markUpdateStarted(fromVersion: string, fromCommit: string) {
    try {
        const v: StartedUpdate = { fromVersion, fromCommit, startedAt: Date.now() };
        sessionStorage.setItem(KEY, JSON.stringify(v));
    } catch {
        /* storage unavailable: the dialog still tracks the job while open */
    }
}

export function readUpdateStarted(): StartedUpdate | null {
    try {
        const raw = sessionStorage.getItem(KEY);
        if (!raw) return null;
        const v = JSON.parse(raw) as StartedUpdate;
        // An entry older than an hour is a job nobody is waiting on any more.
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
