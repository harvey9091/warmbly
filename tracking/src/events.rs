use serde::Serialize;

/// A tracking event published to the event bus. The JSON field names match the
/// Go `events.TrackingEvent` struct tags so the consumer decodes it whether it
/// arrives as JSON (NATS) or Avro (Kafka).
#[derive(Debug, Serialize, Clone)]
pub struct TrackingEvent {
    pub event_type: String,
    pub task_id: String,
    pub original_url: Option<String>,
    /// Click ticket id, so the consumer can name the link (destination and
    /// anchor text) without matching URLs.
    pub link_id: Option<String>,
    pub timestamp: String,
    pub user_agent: Option<String>,
    pub ip_hash: Option<String>,
    /// The source network (last IPv4 octet zeroed, IPv6 cut to 48 bits),
    /// enough for the consumer's location lookup without naming a host.
    pub client_ip: Option<String>,
    /// Names the known-scanner source the request came from, when it came
    /// from one: a mail-filtering network rather than a person's own device.
    /// The consumer records such an event as a machine open or click. None
    /// for every ordinary request, and absent from events written before the
    /// field existed.
    pub scanner: Option<String>,
}
