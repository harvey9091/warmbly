import React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import toast from "react-hot-toast";
import {
    getViewPreferences,
    resetViewPreferences,
    updateViewPreferences,
} from "@/lib/api/client/app/views/views";
import type {
    UpdateViewPreferences,
    ViewName,
    ViewPreferences,
} from "@/lib/api/models/app/views/ViewPreferences";

const queryKey = (orgId: string, view: ViewName) => ["views", orgId, view];
const cacheKey = (orgId: string, view: ViewName) => `warmbly:view:${orgId}:${view}`;

// The server is the source of truth; the browser keeps a copy so the list
// paints with the member's columns on the first frame instead of flashing the
// defaults while the request is in flight. Anything malformed is ignored.
export function readCachedView(orgId: string, view: ViewName): ViewPreferences | undefined {
    if (!orgId) return undefined;
    try {
        const raw = localStorage.getItem(cacheKey(orgId, view));
        if (!raw) return undefined;
        const p = JSON.parse(raw) as Partial<ViewPreferences> | null;
        if (!p || !Array.isArray(p.columns)) return undefined;
        const sort =
            p.sort && typeof p.sort.by === "string" && p.sort.by
                ? { by: p.sort.by, reverse: !!p.sort.reverse }
                : null;
        return { view, columns: p.columns.filter((c): c is string => typeof c === "string"), sort };
    } catch {
        return undefined;
    }
}

function writeCachedView(orgId: string, view: ViewName, prefs: ViewPreferences | null) {
    if (!orgId) return;
    try {
        if (!prefs) localStorage.removeItem(cacheKey(orgId, view));
        else localStorage.setItem(cacheKey(orgId, view), JSON.stringify({ columns: prefs.columns, sort: prefs.sort ?? null }));
    } catch {
        /* storage full or blocked: the server copy still applies */
    }
}

export function useViewPreferences(view: ViewName, orgId: string) {
    const q = useQuery({
        queryKey: queryKey(orgId, view),
        queryFn: async () => (await getViewPreferences(view)).preferences,
        enabled: !!orgId,
        staleTime: 5 * 60 * 1000,
        placeholderData: () => readCachedView(orgId, view),
    });
    React.useEffect(() => {
        if (q.data && !q.isPlaceholderData) writeCachedView(orgId, view, q.data);
    }, [q.data, q.isPlaceholderData, orgId, view]);
    return q;
}

// Optimistic: the table re-lays out on the click, and the row is written
// behind it. A failure puts the previous layout back and says so.
export function useUpdateViewPreferences(view: ViewName, orgId: string) {
    const qc = useQueryClient();
    const key = queryKey(orgId, view);
    return useMutation({
        mutationFn: (body: UpdateViewPreferences) => updateViewPreferences(view, body),
        onMutate: async (body) => {
            await qc.cancelQueries({ queryKey: key });
            const previous = qc.getQueryData<ViewPreferences>(key);
            const next: ViewPreferences = { view, columns: body.columns, sort: body.sort };
            qc.setQueryData<ViewPreferences>(key, next);
            writeCachedView(orgId, view, next);
            return { previous };
        },
        onError: (_err, _body, ctx) => {
            qc.setQueryData(key, ctx?.previous);
            writeCachedView(orgId, view, ctx?.previous ?? null);
            toast.error("Couldn't save your view. It will reset when you reload.");
        },
        onSuccess: (envelope) => {
            qc.setQueryData(key, envelope.preferences);
            writeCachedView(orgId, view, envelope.preferences);
        },
    });
}

export function useResetViewPreferences(view: ViewName, orgId: string) {
    const qc = useQueryClient();
    const key = queryKey(orgId, view);
    return useMutation({
        mutationFn: () => resetViewPreferences(view),
        onMutate: async () => {
            await qc.cancelQueries({ queryKey: key });
            const previous = qc.getQueryData<ViewPreferences>(key);
            qc.setQueryData<ViewPreferences>(key, { view, columns: [], sort: null });
            writeCachedView(orgId, view, null);
            return { previous };
        },
        onError: (_err, _v, ctx) => {
            qc.setQueryData(key, ctx?.previous);
            writeCachedView(orgId, view, ctx?.previous ?? null);
            toast.error("Couldn't reset the view.");
        },
    });
}
