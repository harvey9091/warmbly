import Request from "../../Request";

// PATCH /unibox/folder re-files messages into one canonical folder. Delete in
// the thread header is folder "trash", Archive is "archive". Store-side only:
// the provider copy stays put.
export default async function moveFolder(data: { ids: string[]; folder: "trash" | "archive" | "inbox" }): Promise<void> {
    return await Request<void>({
        method: "PATCH",
        url: `/unibox/folder`,
        data: { email_ids: data.ids, folder: data.folder },
        authorization: true,
    })
}
