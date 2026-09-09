import type EmailImage from "@/lib/api/models/app/campaigns/EmailImage";
import type { EmailImagePage } from "@/lib/api/models/app/campaigns/EmailImage";
import Request from "../../Request";

// Workspace image library for email bodies. The upload is multipart (the file
// rides the "file" field), mirroring campaign attachments; list and delete are
// plain JSON requests. The list is keyset-paginated with an opaque cursor.

export async function listEmailImages(cursor?: string): Promise<EmailImagePage> {
    const res = await Request<EmailImagePage>({
        method: "GET",
        url: cursor ? `/email-images?cursor=${encodeURIComponent(cursor)}` : "/email-images",
        authorization: true,
    });
    return { data: res.data ?? [], pagination: res.pagination ?? { has_more: false } };
}

export async function uploadEmailImage(file: File): Promise<EmailImage> {
    const fd = new FormData();
    fd.append("file", file, file.name);
    return await Request<EmailImage>({
        method: "POST",
        url: "/email-images",
        data: fd,
        authorization: true,
    });
}

export async function deleteEmailImage(id: string): Promise<void> {
    await Request<void>({
        method: "DELETE",
        url: `/email-images/${id}`,
        authorization: true,
    });
}
