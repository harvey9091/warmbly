import Request from "../../Request";

// Switches open/click tracking on a mailbox's hand-written unibox sends.
// Affects mail sent from now on only; anything already delivered keeps
// whatever it carried when it left.
export default async function setDirectTracking(
    emailAccountID: string,
    enabled: boolean,
): Promise<{ track_direct_mail: boolean }> {
    return await Request<{ track_direct_mail: boolean }>({
        method: "PATCH",
        url: `/emails/${encodeURIComponent(emailAccountID)}/direct-tracking`,
        data: { enabled },
        authorization: true,
    })
}
