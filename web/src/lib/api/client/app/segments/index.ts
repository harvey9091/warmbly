import Request from "../../Request";
import type Segment from "@/lib/api/models/app/segments/Segment";
import type {
    CampaignSegmentLink,
    ContactSegment,
    SegmentOverride,
    SegmentAddToCampaignResult,
    SegmentFieldSpec,
    SegmentMemberMode,
    SegmentPreview,
    SegmentWrite,
} from "@/lib/api/models/app/segments/Segment";

export async function listSegments(): Promise<Segment[]> {
    const res = await Request<{ data: Segment[] }>({ method: "GET", url: "/segments", authorization: true });
    return res.data ?? [];
}

export async function getSegment(id: string): Promise<Segment> {
    return await Request<Segment>({ method: "GET", url: `/segments/${id}`, authorization: true });
}

export async function listSegmentFields(): Promise<SegmentFieldSpec[]> {
    const res = await Request<{ data: SegmentFieldSpec[] }>({ method: "GET", url: "/segments/fields", authorization: true });
    return res.data ?? [];
}

export async function createSegment(data: SegmentWrite): Promise<Segment> {
    return await Request<Segment>({ method: "POST", url: "/segments", data, authorization: true });
}

export async function updateSegment(id: string, data: SegmentWrite): Promise<Segment> {
    return await Request<Segment>({ method: "PATCH", url: `/segments/${id}`, data, authorization: true });
}

export async function deleteSegment(id: string): Promise<void> {
    await Request<void>({ method: "DELETE", url: `/segments/${id}`, authorization: true });
}

export async function previewSegment(data: SegmentPreview): Promise<number> {
    const res = await Request<{ contact_count: number }>({ method: "POST", url: "/segments/preview", data, authorization: true });
    return res.contact_count;
}

export async function setSegmentMembers(id: string, contacts: string[], mode: SegmentMemberMode): Promise<number> {
    const res = await Request<{ updated: number }>({
        method: "POST",
        url: `/segments/${id}/members`,
        data: { contacts, mode },
        authorization: true,
    });
    return res.updated;
}

export async function lookupSegmentMembers(id: string, contacts: string[]): Promise<Record<string, SegmentMemberMode>> {
    const res = await Request<{ data: Record<string, SegmentMemberMode> }>({
        method: "POST",
        url: `/segments/${id}/members/lookup`,
        data: { contacts },
        authorization: true,
    });
    return res.data ?? {};
}

export async function listContactSegments(contactId: string): Promise<ContactSegment[]> {
    const res = await Request<{ data: ContactSegment[] }>({ method: "GET", url: `/contacts/${contactId}/segments`, authorization: true });
    return res.data ?? [];
}

export async function listSegmentOverrides(id: string): Promise<SegmentOverride[]> {
    const res = await Request<{ data: SegmentOverride[] }>({ method: "GET", url: `/segments/${id}/overrides`, authorization: true });
    return res.data ?? [];
}

export async function listCampaignSegments(campaignId: string): Promise<CampaignSegmentLink[]> {
    const res = await Request<{ data: CampaignSegmentLink[] }>({
        method: "GET",
        url: `/campaigns/${campaignId}/segments`,
        authorization: true,
    });
    return res.data ?? [];
}

export async function setCampaignSegments(
    campaignId: string,
    segmentIds: string[],
): Promise<{ data: CampaignSegmentLink[]; added: number }> {
    return await Request<{ data: CampaignSegmentLink[]; added: number }>({
        method: "PUT",
        url: `/campaigns/${campaignId}/segments`,
        data: { segment_ids: segmentIds },
        authorization: true,
    });
}

export async function addSegmentToCampaign(id: string, campaignId: string): Promise<SegmentAddToCampaignResult> {
    return await Request<SegmentAddToCampaignResult>({
        method: "POST",
        url: `/segments/${id}/add-to-campaign`,
        data: { campaign_id: campaignId },
        authorization: true,
    });
}
