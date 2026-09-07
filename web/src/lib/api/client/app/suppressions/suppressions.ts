import type {
    AddSuppressionsRequest,
    AddSuppressionsResult,
    SuppressionListResult,
} from "@/lib/api/models/app/suppressions/Suppression";
import Request from "../../Request";

export async function listSuppressions(q: string, cursor: string | null, limit: number): Promise<SuppressionListResult> {
    const params = new URLSearchParams();
    params.set("limit", String(limit));
    if (q) params.set("q", q);
    if (cursor) params.set("cursor", cursor);
    return await Request<SuppressionListResult>({
        method: "GET",
        url: `/suppressions?${params.toString()}`,
        authorization: true,
    });
}

export async function addSuppressions(data: AddSuppressionsRequest): Promise<AddSuppressionsResult> {
    return await Request<AddSuppressionsResult>({
        method: "POST",
        url: "/suppressions",
        data,
        authorization: true,
    });
}

export async function removeSuppression(id: string): Promise<void> {
    await Request<void>({
        method: "DELETE",
        url: `/suppressions/${id}`,
        authorization: true,
    });
}
