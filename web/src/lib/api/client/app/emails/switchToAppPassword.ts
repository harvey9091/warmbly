import Request from "../../Request";
import type Inbox from "@/lib/api/models/app/emails/Inbox";

// Moves a Google sign-in mailbox onto Gmail IMAP and SMTP with an app password, in place; checked live with Gmail first.
export default async function switchToAppPassword(id: string, appPassword: string): Promise<Inbox> {
    return await Request<Inbox>({
        method: "POST",
        url: `/emails/onboarding/app-password/${id}`,
        data: { app_password: appPassword },
        authorization: true,
        timeout: 60_000,
    });
}
