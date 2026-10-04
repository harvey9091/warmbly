import Request from "../../Request";

// Replaces the folders a mailbox's sync leaves alone. The list is the
// desired state, so a retry converges.
export default async function updateSync(id: string, skipFolders: string[]): Promise<{ skip_folders: string[] }> {
    return await Request<{ skip_folders: string[] }>({
        method: "PUT",
        url: `/emails/${id}/sync`,
        data: { skip_folders: skipFolders },
        authorization: true,
    });
}
