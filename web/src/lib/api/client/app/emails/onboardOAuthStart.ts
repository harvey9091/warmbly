import Request from "../../Request";

export interface OAuthStartResponse {
    url: string;
    state: string;
}

// loginHint pre-selects the account on the provider's consent screen.
export default async function onboardOAuthStart(provider: "gmail" | "outlook", loginHint?: string): Promise<OAuthStartResponse> {
    return await Request<OAuthStartResponse>({
        method: "POST",
        url: `/emails/onboarding/oauth/start`,
        data: loginHint ? { provider, login_hint: loginHint } : { provider },
        authorization: true,
    });
}
