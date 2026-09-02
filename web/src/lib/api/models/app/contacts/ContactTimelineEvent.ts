export type ContactTimelineEventType =
    | "email_sent"
    | "email_opened"
    | "email_clicked"
    | "email_replied"
    | "email_bounced"
    | "reply_received"
    | "deliverability"
    | "suppressed"
    | "note"
    | "meeting_booked"
    | "meeting_rescheduled"
    | "meeting_canceled"
    // Lifecycle (from contact_activities). contact_created carries the
    // first-touch source, which is how "imported" and "created via API"
    // are told apart.
    | "contact_created"
    | "campaign_added"
    | "campaign_removed"
    | "category_added"
    | "category_removed"
    | "form_submitted"
    | "page_hit";

// A page view from the website tracking snippet (page_hit).
export interface ContactPageHit {
    id: string;
    visitor_id: string;
    session_key: string;
    occurred_at: string;
    url: string;
    path: string;
    title: string;
    referrer: string;
    referrer_domain: string;
    landing: boolean;
    utm_source: string;
    utm_medium: string;
    utm_campaign: string;
    utm_term: string;
    utm_content: string;
    device_type: string;
    os: string;
    browser: string;
    browser_version: string;
    device_brand: string;
    language: string;
    timezone: string;
    screen_width: number;
    screen_height: number;
    country_code: string;
    region: string;
    city: string;
}

export default interface ContactTimelineEvent {
    type: ContactTimelineEventType;
    at: string;

    email_account_id?: string | null;
    email_account_email?: string | null;
    email_account_name?: string | null;

    campaign_id?: string | null;
    campaign_name?: string | null;
    step_id?: string | null;
    step_name?: string | null;

    task_id?: string | null;
    subject?: string | null;

    reason?: string | null;
    source?: string | null;
    provider?: string | null;
    intent?: string | null;
    content?: string | null;

    // Meeting events (meeting_booked / rescheduled / canceled).
    scheduled_for?: string | null;
    join_url?: string | null;
    meeting_state?: string | null;

    // Lifecycle events.
    category_id?: string | null;
    category_title?: string | null;
    form_id?: string | null;
    form_name?: string | null;
    source_detail?: string | null;

    user_id?: string | null;

    // Website page view (page_hit).
    page_hit?: ContactPageHit | null;
}

export interface ContactTimelineResult {
    data: ContactTimelineEvent[];
    has_more: boolean;
}
