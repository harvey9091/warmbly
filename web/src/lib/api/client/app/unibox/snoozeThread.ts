import Request from "../../Request";

export interface SnoozeRequest {
    thread_id: string;
    /** RFC3339 timestamp — the snooze releases when this passes. */
    snoozed_until: string;
}

export interface SnoozeResponse {
    id: string;
    thread_id: string;
    snoozed_until: string;
}

export async function snoozeThread(req: SnoozeRequest): Promise<SnoozeResponse> {
    return await Request<SnoozeResponse>({
        method: "POST",
        url: "/unibox/snooze",
        authorization: true,
        data: req,
    });
}

// The selection bar's form: one call for a whole selection rather than one per
// row. The server answers with `data` when several are named.
export async function snoozeThreads(
    threadIds: string[],
    until: Date,
): Promise<void> {
    await Request<{ data: SnoozeResponse[] }>({
        method: "POST",
        url: "/unibox/snooze",
        authorization: true,
        data: { thread_ids: threadIds, snoozed_until: until.toISOString() },
    });
}

export async function unsnoozeThread(threadId: string): Promise<void> {
    return await unsnoozeThreads([threadId]);
}

export async function unsnoozeThreads(threadIds: string[]): Promise<void> {
    const usp = new URLSearchParams({ thread_id: threadIds.join(",") });
    await Request<void>({
        method: "DELETE",
        url: `/unibox/snooze?${usp.toString()}`,
        authorization: true,
    });
}
