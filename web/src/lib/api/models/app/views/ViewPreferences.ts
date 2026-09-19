// A member's own layout of one dashboard list in the current workspace: the
// columns shown (in order, Name implied first) and the sort. Mirrors
// models.ViewPreferences on the backend.

export type ViewName = "contacts" | "campaign_leads";

export interface ViewSort {
    by: string;
    reverse: boolean;
}

export interface ViewPreferences {
    view: ViewName;
    // Column ids in display order. Empty means the list's default layout.
    columns: string[];
    // Null or absent means the list's default sort.
    sort?: ViewSort | null;
    updated_at?: string | null;
}

export interface ViewPreferencesEnvelope {
    preferences: ViewPreferences;
}

export interface UpdateViewPreferences {
    columns: string[];
    sort: ViewSort | null;
}
