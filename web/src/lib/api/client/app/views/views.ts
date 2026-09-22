import type {
    UpdateViewPreferences,
    ViewName,
    ViewPreferencesEnvelope,
} from "@/lib/api/models/app/views/ViewPreferences";
import Request from "../../Request";

// GET /me/views/:view — the caller's saved layout for one list, or the empty
// layout when none is saved. Scoped to the current workspace by the org header.
export async function getViewPreferences(view: ViewName): Promise<ViewPreferencesEnvelope> {
    return await Request<ViewPreferencesEnvelope>({
        method: "GET",
        url: `/me/views/${view}`,
        authorization: true,
    });
}

export async function updateViewPreferences(view: ViewName, body: UpdateViewPreferences): Promise<ViewPreferencesEnvelope> {
    return await Request<ViewPreferencesEnvelope>({
        method: "PUT",
        url: `/me/views/${view}`,
        data: body,
        authorization: true,
    });
}

export async function resetViewPreferences(view: ViewName): Promise<void> {
    await Request<void>({
        method: "DELETE",
        url: `/me/views/${view}`,
        authorization: true,
    });
}
