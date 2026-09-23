import Request from "../../../Request";

// The failed rows as uploaded, without secrets, plus error and fix columns.
export default async function downloadMailboxImportFailed(id: string): Promise<Blob> {
    return await Request<Blob>({
        method: "GET",
        url: `/emails/imports/${id}/failed.csv`,
        authorization: true,
        responseType: "blob",
    });
}
