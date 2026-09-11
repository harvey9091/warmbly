/**
 * What this deployment's auth can actually do, served by GET /auth/config.
 *
 * The login screen used to guess: it mounted the Turnstile widget in every
 * production build even when captcha was off server-side, rendered social
 * buttons for providers the backend had no client for, and gave a self-hoster
 * no hint that their login code had gone to a log file.
 */
export default interface AuthConfig {
    captcha: boolean;
    password_login: boolean;
    login_code: "always" | "new_device" | "off";
    registration: "true" | "false" | "invite_only";
    email_verification: boolean;
    mail_delivers: boolean;
    passkeys: boolean;
    providers: string[];
    /** What each provider's button should say, keyed by the same identifiers.
     *  Absent for a backend that predates it, so callers fall back to a name
     *  of their own. */
    provider_labels?: Record<string, string>;
    self_hosted: boolean;
    /** False when BILLING_PROVIDER=none: the backend unlocks every feature and
     *  the org must not be presented as being on a trial or free tier. */
    billing_enabled: boolean;
    /** True while the instance has no accounts and must be claimed. */
    setup_required: boolean;
    /** registration === "invite_only", precomputed by the server so the client
     *  does not reimplement the meaning of a tri-state string. */
    invites_required: boolean;
    /** Where to send someone a deployment policy refused, not their mistake. */
    docs_url: string;
    /** The dashboard origin this deployment builds its emailed links from. */
    app_url?: string;
    /** This API's own public base, for copyable API examples. Absent on a
     *  backend that predates it, in which case examples fall back to the API
     *  origin the dashboard itself is configured with. */
    api_url?: string;
    /** Who this deployment says it is. Every field but the name is absent on a
     *  self-host that configured no EMAIL_BRAND_*, and the UI then renders no
     *  link at all rather than sending the operator's users to warmbly.com. */
    brand?: DeploymentBrand;
}

export interface DeploymentBrand {
    name: string;
    website_url?: string;
    website_label?: string;
    terms_url?: string;
    privacy_url?: string;
    support_email?: string;
}
