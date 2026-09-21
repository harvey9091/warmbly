import type { LoginResult } from "../../models/auth/LoginResult";
import Request from "../Request";

/**
 * Completes a federated sign-in that came back `link_required`: the address
 * the provider asserted already belongs to an account with a password, and
 * that password is what attaches the provider identity to it. Public, like
 * the 2FA verify: the pending token is the proof of the completed provider
 * flow. The result can still be a 2FA challenge.
 */
export default async function linkSSO(pending_token: string, password: string): Promise<LoginResult> {
    return await Request<LoginResult>({
        method: "POST",
        url: "/auth/sso/link",
        data: { pending_token, password },
    });
}
