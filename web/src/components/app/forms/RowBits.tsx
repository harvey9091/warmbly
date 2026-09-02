// Small row primitives shared by the responses table and the analytics tab so
// a person reads the same way on both.

export function InitialsAvatar({ name, email }: { name?: string; email?: string }) {
    const source = (name || email || "").trim();
    const words = source.split(/\s+/).filter(Boolean);
    const initials =
        words.length >= 2
            ? (words[0][0] + words[words.length - 1][0]).toUpperCase()
            : source.slice(0, 2).toUpperCase() || "?";
    return (
        <div className="size-7 rounded-full bg-slate-100 text-slate-600 text-[11px] font-medium flex items-center justify-center shrink-0">
            {initials}
        </div>
    );
}

export function MiniProgress({ done, total }: { done: number; total: number }) {
    const clamped = Math.min(done, total);
    return (
        <div className="flex items-center gap-2">
            <div className="w-24 h-1.5 rounded-full bg-slate-100 overflow-hidden">
                <div
                    className="h-full rounded-full bg-sky-500"
                    style={{ width: `${total > 0 ? (clamped / total) * 100 : 0}%` }}
                />
            </div>
            <span className="font-mono text-[11px] tabular-nums text-slate-500">
                {clamped}/{total}
            </span>
        </div>
    );
}

export function StatusPill({ completed }: { completed: boolean }) {
    return (
        <span
            className={`inline-flex items-center gap-1.5 h-5 px-2 rounded-full text-[11px] font-medium whitespace-nowrap ${
                completed ? "bg-emerald-50 text-emerald-700" : "bg-amber-50 text-amber-700"
            }`}
        >
            <span className={`w-1.5 h-1.5 rounded-full ${completed ? "bg-emerald-500" : "bg-amber-500"}`} />
            {completed ? "Completed" : "In progress"}
        </span>
    );
}
