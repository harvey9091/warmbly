import Request from "../../Request";

// Permanent removal, distinct from revoking: the row and its usage logs go.
// The backend refuses a key that can still authenticate.
export default async function deleteAPIKey(id: string): Promise<void> {
    return await Request<void>({
        method: "DELETE",
        url: `/api-keys/${id}/permanent`,
        authorization: true,
    });
}
