// /admin/workers/* — what a worker is carrying.
//
// The machine half of a worker (liveness, version, usage, enrolment) lives in
// the fleet node API instead; see ./fleetNodes.ts. There is nothing here that
// reaches into a machine, because nothing does any more.

import { Request } from "@/lib/api/client";
import type { AdminWorkerEmailsResult } from "@/lib/api/models/admin";

// getWorkerEmails returns the mailboxes assigned to a worker (paginated), with
// per-mailbox risk band + warmup health so the detail page can show how healthy
// the inboxes on this worker are.
export function getWorkerEmails(
    id: string,
    cursor?: string,
): Promise<AdminWorkerEmailsResult> {
    const q = cursor ? `?cursor=${cursor}` : "";
    return Request({
        method: "GET",
        url: `/admin/workers/${id}/emails${q}`,
        authorization: true,
    });
}

// Mirrors models.WorkerStats. Renaming a field here does not rename it on the
// wire; it just renders blank.
export interface WorkerStats {
    worker_id: string;
    total_emails_sent: number;
    emails_sent_today: number;
    emails_sent_this_week: number;
    average_delivery_time_ms: number;
    success_rate: number;
    queue_depth: number;
}

export function getWorkerStats(id: string): Promise<WorkerStats> {
    return Request({
        method: "GET",
        url: `/admin/workers/${id}/stats`,
        authorization: true,
    });
}

// Moves mailboxes onto the worker in the URL. Placement would get there on its
// own, so this is for when you know something it does not; the rotation loop
// will move them again if it disagrees once their residency window passes.
export function reassignWorkerEmails(
    targetWorkerId: string,
    emailIds: string[],
): Promise<{ ok: boolean }> {
    return Request({
        method: "POST",
        url: `/admin/workers/${targetWorkerId}/reassign`,
        // The target is in the URL and in the body: models.ReassignEmailsRequest
        // marks new_worker_id required, so omitting it is a 400.
        data: { email_ids: emailIds, new_worker_id: targetWorkerId },
        authorization: true,
    });
}

export function setWorkerTags(id: string, tags: string[]): Promise<{ ok: boolean; tags: string[] }> {
    return Request({
        method: "PUT",
        url: `/admin/workers/${id}/tags`,
        data: { tags },
        authorization: true,
    });
}

export function listWorkerTags(): Promise<{ data: string[] }> {
    return Request({ method: "GET", url: "/admin/workers/tags", authorization: true });
}
