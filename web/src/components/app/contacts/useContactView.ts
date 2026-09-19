// The member's saved layout of a contacts list, resolved against the column
// registry and the workspace's custom fields. One hook, so the table, the
// chooser and the sort menu read the same view.

import React from "react";
import { useCurrentOrg } from "@/stores/useAppStore";
import useCustomFieldKeys from "@/lib/api/hooks/app/contacts/useCustomFieldKeys";
import {
    useResetViewPreferences,
    useUpdateViewPreferences,
    useViewPreferences,
} from "@/lib/api/hooks/app/views/useViewPreferences";
import type { ViewName, ViewSort } from "@/lib/api/models/app/views/ViewPreferences";
import { resolveColumns } from "./columns";

export function useContactView(view: ViewName) {
    const org = useCurrentOrg();
    const orgId = org?.id ?? "";
    const prefs = useViewPreferences(view, orgId);
    const update = useUpdateViewPreferences(view, orgId);
    const reset = useResetViewPreferences(view, orgId);
    const keys = useCustomFieldKeys();

    const saved = prefs.data;
    const savedColumns = saved?.columns;
    const customKeys = React.useMemo(() => keys.data ?? [], [keys.data]);
    const { visible, available } = React.useMemo(
        () => resolveColumns(view, savedColumns, customKeys),
        [view, savedColumns, customKeys],
    );

    const setColumns = React.useCallback(
        (ids: string[]) => update.mutate({ columns: ids, sort: saved?.sort ?? null }),
        [update, saved?.sort],
    );
    const setSort = React.useCallback(
        (sort: ViewSort) => update.mutate({ columns: saved?.columns ?? [], sort }),
        [update, saved?.columns],
    );

    return {
        orgId,
        columns: visible,
        available,
        customKeys,
        savedSort: saved?.sort ?? null,
        // True once the server's copy is in hand (not the browser's cached one).
        loaded: prefs.isSuccess && !prefs.isPlaceholderData,
        customized: (savedColumns?.length ?? 0) > 0,
        setColumns,
        setSort,
        reset: () => reset.mutate(),
    };
}
