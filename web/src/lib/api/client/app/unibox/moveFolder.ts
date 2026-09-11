import Request from "../../Request";

// The three folders a user can file a conversation into. sent/drafts/spam are
// verdicts the provider reaches, and the backend refuses them here.
export type FilableFolder = "inbox" | "archive" | "trash";

// PATCH /unibox/folder re-files messages. Archive in the thread header is
// "archive", Delete is "trash", Move to inbox is "inbox". Store-side only: the
// provider copy stays put, and the sync knows not to undo it.
export default async function moveFolder(data: { ids: string[]; folder: FilableFolder }): Promise<void> {
    return await Request<void>({
        method: "PATCH",
        url: `/unibox/folder`,
        data: { email_ids: data.ids, folder: data.folder },
        authorization: true,
    })
}
