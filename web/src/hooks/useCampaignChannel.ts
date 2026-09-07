import { useState, useCallback, useEffect, useRef } from 'react';
import { useChannel, useChannelEvent } from './context/socket';

// Task progress event payload
export interface TaskProgressPayload {
    campaign_id: string;
    task_id: string;
    status: 'pending' | 'active' | 'completed' | 'failed';
    contact_id: string;
    contact_email: string;
    contact_name: string;
    step_id: string;
    step_name: string;
    step_index: number;
    progress: number;
    total_contacts: number;
    processed_count: number;
    timestamp: string;
}

// Campaign status event payload
export interface CampaignStatusPayload {
    campaign_id: string;
    status: 'draft' | 'active' | 'paused' | 'completed';
    name?: string;
}

// Activity item for the feed
export interface ActivityItem {
    id: string;
    type: 'sent' | 'opened' | 'clicked' | 'replied' | 'bounced' | 'failed';
    contactEmail: string;
    contactName?: string;
    message: string;
    timestamp: Date;
}

// Open/click payload (pubsub.TrackingEventPayload): who, whether a person
// did it, and what the logs say about client and location.
interface TrackingPayload {
    contact_email?: string;
    original_url?: string;
    link_label?: string;
    machine?: boolean;
    // When the tracking service saw it; the envelope timestamp is the publish time.
    occurred_at?: string;
    timestamp?: string;
    client?: string;
    device_type?: string;
    country_code?: string;
    city?: string;
}

// The edge's time when it is carried and parses; receipt time otherwise.
function occurredAt(p: TrackingPayload): Date {
    const raw = p.occurred_at || p.timestamp;
    const d = raw ? new Date(raw) : null;
    return d && !Number.isNaN(d.getTime()) ? d : new Date();
}

// " (Gmail, Berlin DE)" or "" when the logs say nothing.
function whereFrom(p: TrackingPayload): string {
    const bits = [p.client, [p.city, p.country_code].filter(Boolean).join(' ')].filter(Boolean);
    return bits.length ? ` (${bits.join(', ')})` : '';
}

// Hook return type
export interface CampaignChannelState {
    isConnected: boolean;
    channelState: string;
    campaignStatus: CampaignStatusPayload | null;
    taskProgress: TaskProgressPayload | null;
    activities: ActivityItem[];
    clearActivities: () => void;
}

const MAX_ACTIVITIES = 50;

// Normalize a realtime event name to a canonical UPPERCASE_SNAKE key. The Go
// publishers set `event_type` to uppercase constants (TASK_PROGRESS,
// EMAIL_OPENED, CAMPAIGN_STARTED, …) and the Elixir channel pushes that verbatim
// as the Phoenix event name. Normalizing here (the same way useRealtimeEvents
// does) means the feed works regardless of any future casing/separator drift.
function normalizeEvent(name: unknown): string {
    return String(name ?? '')
        .replace(/[.:\s-]+/g, '_')
        .toUpperCase();
}

export function useCampaignChannel(campaignId: string): CampaignChannelState {
    const topic = `campaign:${campaignId}`;
    const channel = useChannel(topic);

    const [campaignStatus, setCampaignStatus] = useState<CampaignStatusPayload | null>(null);
    const [taskProgress, setTaskProgress] = useState<TaskProgressPayload | null>(null);
    const [activities, setActivities] = useState<ActivityItem[]>([]);

    // Monotonic id source so rapid same-millisecond events don't collide on
    // React keys (Date.now() alone repeats under bursty sending).
    const idRef = useRef(0);
    const nextId = useCallback((prefix: string) => `${prefix}-${++idRef.current}`, []);

    // Add activity to feed
    const addActivity = useCallback((activity: ActivityItem) => {
        setActivities((prev) => {
            const newActivities = [activity, ...prev];
            return newActivities.slice(0, MAX_ACTIVITIES);
        });
    }, []);

    // Clear activities
    const clearActivities = useCallback(() => {
        setActivities([]);
    }, []);

    // Single wildcard subscription: the socket dispatcher passes every event on
    // this channel here with `_event` set to the Phoenix event name. We
    // normalize and route, instead of registering lowercase handlers that never
    // matched the uppercase event names the backend actually emits.
    useChannelEvent(topic, '*', (payload) => {
        const raw = payload as { _event?: string; event_type?: string; type?: string };
        const name = normalizeEvent(raw._event ?? raw.event_type ?? raw.type);

        switch (name) {
            case 'TASK_PROGRESS': {
                const data = payload as unknown as TaskProgressPayload;
                setTaskProgress(data);
                if (data.status === 'completed') {
                    addActivity({
                        id: nextId('sent'),
                        type: 'sent',
                        contactEmail: data.contact_email,
                        contactName: data.contact_name,
                        message: `Email sent to ${data.contact_email}`,
                        timestamp: new Date(data.timestamp || Date.now()),
                    });
                } else if (data.status === 'failed') {
                    addActivity({
                        id: nextId('failed'),
                        type: 'failed',
                        contactEmail: data.contact_email,
                        contactName: data.contact_name,
                        message: `Failed to send to ${data.contact_email}`,
                        timestamp: new Date(data.timestamp || Date.now()),
                    });
                }
                break;
            }
            case 'EMAIL_OPENED': {
                const data = payload as TrackingPayload;
                addActivity({
                    id: nextId('open'),
                    type: 'opened',
                    contactEmail: data.contact_email || 'Unknown',
                    message: `${data.machine ? 'Auto-opened' : 'Opened'} by ${data.contact_email || 'Unknown'}${whereFrom(data)}`,
                    timestamp: occurredAt(data),
                });
                break;
            }
            case 'EMAIL_CLICKED': {
                const data = payload as TrackingPayload;
                const who = `${data.contact_email || 'Unknown'}${whereFrom(data)}`;
                const target = data.link_label || data.original_url;
                addActivity({
                    id: nextId('click'),
                    type: 'clicked',
                    contactEmail: data.contact_email || 'Unknown',
                    message: data.machine
                        ? `Automated click on ${target ?? 'a link'} for ${who} (not counted)`
                        : target
                          ? `Click from ${who} → ${target}`
                          : `Click from ${who}`,
                    timestamp: occurredAt(data),
                });
                break;
            }
            case 'EMAIL_REPLIED': {
                const data = payload as { contact_email?: string };
                addActivity({
                    id: nextId('reply'),
                    type: 'replied',
                    contactEmail: data.contact_email || 'Unknown',
                    message: `Reply from ${data.contact_email || 'Unknown'}`,
                    timestamp: new Date(),
                });
                break;
            }
            case 'EMAIL_BOUNCED': {
                const data = payload as { contact_email?: string };
                addActivity({
                    id: nextId('bounce'),
                    type: 'bounced',
                    contactEmail: data.contact_email || 'Unknown',
                    message: `Bounced: ${data.contact_email || 'Unknown'}`,
                    timestamp: new Date(),
                });
                break;
            }
            case 'CAMPAIGN_STARTED': {
                const data = payload as unknown as CampaignStatusPayload;
                setCampaignStatus({ ...data, status: 'active' });
                break;
            }
            case 'CAMPAIGN_PAUSED': {
                const data = payload as unknown as CampaignStatusPayload;
                setCampaignStatus({ ...data, status: 'paused' });
                break;
            }
            case 'CAMPAIGN_COMPLETED': {
                const data = payload as unknown as CampaignStatusPayload;
                setCampaignStatus({ ...data, status: 'completed' });
                break;
            }
            case 'CAMPAIGN_UPDATED': {
                const data = payload as unknown as CampaignStatusPayload;
                setCampaignStatus(data);
                break;
            }
        }
    });

    // Reset state when campaign changes
    useEffect(() => {
        setCampaignStatus(null);
        setTaskProgress(null);
        setActivities([]);
    }, [campaignId]);

    return {
        isConnected: channel.state === 'joined',
        channelState: channel.state,
        campaignStatus,
        taskProgress,
        activities,
        clearActivities,
    };
}

export default useCampaignChannel;
