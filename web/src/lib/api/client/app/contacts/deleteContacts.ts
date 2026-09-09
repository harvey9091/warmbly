import type ContactSelection from "@/lib/api/models/app/contacts/ContactSelection";
import Request from "../../Request";

// The endpoint also accepts a bare id array (the published shape); the client
// always sends the selection object so "select all matching" is one request
// instead of a page walk.
export default async function deleteContacts(selection: ContactSelection): Promise<void> {
    return await Request<void>({
        method: "DELETE",
        url: "/contacts",
        data: selection,
        authorization: true,
    })
}
