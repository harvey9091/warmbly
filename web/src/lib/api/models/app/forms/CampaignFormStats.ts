// Per-form performance inside one campaign. Mirrors models.CampaignFormStats.

export default interface CampaignFormStats {
    form_id: string;
    form_name: string;
    public_id: string;
    status: "draft" | "published" | "archived";
    /** Recipients who were given a personalized link by this campaign. */
    links_sent: number;
    viewers: number;
    starters: number;
    submissions: number;
    share_url?: string;
}
