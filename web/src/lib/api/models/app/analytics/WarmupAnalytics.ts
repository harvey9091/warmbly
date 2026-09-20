// Per-mailbox warmup analytics from GET /analytics/warmup?email_id=&from=&to=
// (backend models.WarmupAnalytics).

export interface WarmupSummary {
    total_sent: number;
    total_replied: number;
    total_received: number; // verified warmup mail that arrived from partners
    average_daily: number; // per active day in the selected range
    reply_rate: number; // percentage
    target_progress: number; // actual sends / planned target volume
    days_active: number;
}

export interface WarmupDailyStat {
    date: string; // YYYY-MM-DD
    emails_sent: number;
    emails_replied: number;
    emails_received: number;
    target_volume: number;
}

export default interface WarmupAnalytics {
    email_account_id: string;
    email: string;
    date_range: { from: string; to: string };
    summary: WarmupSummary;
    daily_stats: WarmupDailyStat[];
}
