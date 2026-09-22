# Mailbox worker capacity research

Date: 2026-09-17

## Question

What actually limits a Warmbly worker that connects to customer mailboxes, and should the worker's public IP define how many mailboxes it can carry?

This note covers ordinary authenticated mailbox access through the Gmail API, Microsoft Graph, SMTP submission, and IMAP. It does not treat a worker as a bulk mail transfer agent and does not recommend ways to evade provider abuse controls.

## Conclusion

The current flat worker-capacity model has no sound provider-side basis. Google and Microsoft enforce the important limits per mailbox, per user, per tenant, or per API project. Their published limits do not turn 88 separate mailboxes on one worker into 88 connections against one mailbox's quota.

For Gmail API and Microsoft Graph mailboxes, the worker makes HTTPS calls and the provider sends the message from its own outbound mail infrastructure. For authenticated SMTP submission, the worker submits to the mailbox provider, which then relays the message. The recipient-facing SMTP peer is therefore the provider's outbound server. Its IP reputation matters, but Warmbly does not own or select that IP.

The worker's public IP still matters in a different role. Google, Microsoft, and custom mailbox providers can see it as the client that signs in, opens IMAP connections, or submits through SMTP. It can affect authentication challenges, conditional access, IP allowlists, and provider-side throttling. Stable placement and measured backoff are useful for those reasons. That worker IP should not be presented as the recipient-facing sending reputation or converted into a fixed mailbox ceiling.

A worker should be sized from resources Warmbly actually consumes: assigned mailbox state, live sockets, concurrent sync and send work, memory, file descriptors, CPU, network traffic, and observed provider throttles. Sending budgets and pacing belong primarily to the mailbox, domain, provider tenant, and API credential scopes.

## What providers actually limit

### Google Workspace and Gmail

Google Workspace publishes these Gmail user limits:

- 2,000 messages per user per day for paid accounts and 500 for trial accounts.
- 10,000 total recipients per user per day.
- 3,000 external recipients and 2,000 unique external recipients per user per day.
- 500 recipients per message through the Gmail API and 100 recipients per message when submitted through SMTP by a POP or IMAP user.

Google states that limits can be applied over a rolling 24-hour period and that a user who exceeds them can be unable to send for up to 24 hours. These are account limits, not worker-host limits. Source: [Gmail sending limits in Google Workspace](https://knowledge.workspace.google.com/admin/gmail/gmail-sending-limits-in-google-workspace).

The Gmail API separately limits request traffic. As of May 2026, it publishes 1,200,000 quota units per minute per project and 6,000 units per minute per user per project. `messages.send` costs 100 units. The resulting request ceiling is far above a normal paced campaign mailbox, but the project-wide scope matters at a large fleet size. The Gmail user sending limits continue to apply independently. Source: [Gmail API usage limits](https://developers.google.com/workspace/gmail/api/reference/quota).

Gmail documents up to 15 simultaneous email clients per account before it reports too many simultaneous connections. That is a per-account constraint. One connection to each of 88 distinct mailboxes is not 88 connections to one mailbox. Source: [Add Gmail to another email client](https://support.google.com/mail/answer/7126229?hl=en).

Google also documents a distinct SMTP relay product with its own limits, including 10,000 messages and 10,000 unique recipients per user per 24 hours, 100 recipients per SMTP transaction, and organization-wide recipient ceilings. Those relay limits should only be applied to accounts that actually use that product. Source: [Route outgoing SMTP relay messages through Google](https://knowledge.workspace.google.com/admin/gmail/advanced/route-outgoing-smtp-relay-messages-through-google).

Google's sender guidance says to increase volume slowly, keep the rate consistent, avoid bursts, monitor provider responses, and reduce volume when messages bounce or are deferred. It describes volume and reputation scopes at the sending domain and sending IP. These are pacing and deliverability controls, not evidence for a fixed mailbox count per application worker. Source: [Email sender guidelines](https://support.google.com/mail/answer/81126?hl=en-GB).

### Microsoft 365 and Exchange Online

Exchange Online publishes these mailbox sending limits:

- 10,000 recipients per mailbox in a rolling 24-hour window.
- 30 messages per minute.
- A default 500 recipients per message, configurable up to 1,000.
- A tenant external recipient rate limit that depends on the tenant's licenses.

Microsoft calls the per-user recipient and message rates hard service limits and can also restrict a user when it detects outbound spam. It explicitly recommends a specialized service when an organization needs bulk or high-volume external email. These hard limits are upper service bounds, not safe campaign targets. Source: [Troubleshoot outbound sending limits in Exchange Online](https://learn.microsoft.com/en-us/defender-office-365/outbound-spam-sending-limits-troubleshoot) and [Exchange Online limits](https://learn.microsoft.com/en-us/office365/servicedescriptions/exchange-online-service-description/exchange-online-limits).

For SMTP AUTH, Microsoft permits up to three concurrent submission connections per mailbox. It also applies the 30 messages-per-minute and 10,000 recipients-per-day mailbox limits. The three-connection limit is not a three-connection limit for an entire Warmbly worker. A serialized send lane for each mailbox remains well below it. Source: [Message storage and concurrent connection throttling for SMTP Authenticated Submission](https://learn.microsoft.com/en-us/troubleshoot/exchange/send-emails/smtp-submission-improvements).

For Microsoft Graph's Outlook resources, throttling is scoped to an app ID and mailbox combination. Microsoft publishes 10,000 requests per 10 minutes, four concurrent requests, and 150 MB of uploads per five minutes for that combination, and explicitly says that exceeding the limit for one mailbox does not affect another mailbox. Source: [Microsoft Graph service-specific throttling limits](https://learn.microsoft.com/en-us/graph/throttling-limits#outlook-service-limits).

Microsoft publishes configurable IMAP connection limits for self-managed Exchange Server, including a default of 16 connections per user, but that documentation does not establish a universal Exchange Online or generic IMAP-host ceiling. Custom IMAP servers can apply per-user, per-source-IP, and server-wide limits of their own. Warmbly should use one serialized IMAP session per mailbox, respect server errors, and learn from observed throttling rather than encoding an Exchange Server default as a global worker capacity. Source: [Set connection limits for IMAP4 in Exchange Server](https://learn.microsoft.com/en-us/exchange/set-connection-limits-for-imap4-exchange-2013-help) and [Microsoft guidance on IMAP migration connection limits](https://learn.microsoft.com/en-us/exchange/mailbox-migration/migrating-imap-mailboxes/optimizing-imap-migrations).

## Which IP has recipient-facing reputation

SMTP reputation applies to the host that connects to the recipient's receiving mail server. SPF is evaluated with the IP address of the SMTP client emitting mail to that receiver. Source: [RFC 7208, section 4.1](https://www.rfc-editor.org/rfc/rfc7208#section-4.1). SMTP servers also add `Received` trace fields as a message crosses each hop. Source: [RFC 5321, section 4.4](https://www.rfc-editor.org/rfc/rfc5321#section-4.4).

Google tells Workspace customers who send only through Workspace to authorize `_spf.google.com`, and publishes Google Workspace's changing outbound mail-server ranges. Source: [Set up SPF for Google Workspace](https://knowledge.workspace.google.com/admin/security/set-up-spf) and [Google IP address ranges for outbound mail servers](https://knowledge.workspace.google.com/admin/gmail/advanced/google-ip-address-ranges-for-outbound-mail-servers).

Microsoft similarly tells Exchange Online customers to authorize `spf.protection.outlook.com` as the source for Microsoft 365 mail. Source: [Set up SPF for a Microsoft 365 domain](https://learn.microsoft.com/en-us/defender-office-365/email-authentication-spf-configure) and [Microsoft 365 external DNS records](https://learn.microsoft.com/en-us/microsoft-365/enterprise/external-domain-name-system-records).

The resulting distinction is:

| Role | IP that matters | Who controls it | What it affects |
| --- | --- | --- | --- |
| Worker to mailbox provider | Worker egress IP | Warmbly operator | Sign-in risk, conditional access, relay allowlists, connection and authentication throttles |
| Provider to recipient MX | Google, Microsoft, or custom provider outbound IP | Mailbox provider | Recipient-side IP reputation, SPF evaluation, receiver rate limits |
| Direct delivery to recipient MX | Application host IP | Application operator | Recipient-side IP reputation and SPF, but Warmbly's mailbox send paths do not use this architecture |

Saying that IP never matters is too broad. Google explicitly tracks sending-IP and domain reputation. In Warmbly's hosted-mailbox architecture, however, the recipient-facing sending IP is normally the mailbox provider's IP, not the Warmbly worker's egress IP. That is the important correction to the current capacity discussion.

## Campaign pacing implications

Provider hard limits should be enforced as safety ceilings in the scope the provider defines:

- Per mailbox: daily recipients/messages, per-minute sends, connection concurrency, and a serialized send lane.
- Per tenant: Exchange Online's tenant external recipient limit and any customer-admin outbound policy.
- Per API credential or project: Gmail API project quota and any provider application quota.
- Per sending domain: authentication, complaint, bounce, and provider feedback signals.

Campaign pacing should stay per mailbox and domain. A low-volume, consistent lane with adaptive backoff is compatible with official sender guidance. A worker-wide send cap would combine unrelated mailboxes, and raising a worker cap would not raise any mailbox's provider allowance.

The application should respond to `429`, `Retry-After`, SMTP temporary failures, and provider-specific quota errors in the exact scope reported. A mailbox throttle should normally pause that mailbox. A tenant or project throttle must coordinate across workers, which requires shared state rather than a local worker counter.

## Recommended worker model

Use separate measurements instead of one synthetic `load / capacity` ratio.

### Assignment capacity

The placement hard limit should come from operator-configured or measured machine resources:

- maximum assigned mailboxes;
- maximum simultaneous sync operations;
- maximum simultaneous sends;
- maximum live IMAP sockets;
- memory, CPU, file descriptors, and network headroom.

API mailboxes and SMTP/IMAP mailboxes can have different observed local costs, but those costs should be calibrated from worker telemetry. A provider's per-mailbox limit is not evidence that a generic SMTP/IMAP mailbox costs twenty times an API mailbox on the host.

### Provider budgets

Each mailbox should have its own provider-aware send and request governor. Tenant and API-project limits should sit above those mailbox governors. Backoff should be updated from actual quota responses. This preserves the scope of the provider's limit and lets hundreds of low-duty-cycle mailboxes share a capable worker when local resource measurements permit it.

### Placement scoring

Placement should prefer:

- a healthy worker with measured headroom;
- the incumbent worker, to avoid unnecessary sign-in-location changes;
- spreading one organization's mailboxes across failure domains;
- the expected sign-in region when known;
- avoiding a provider concentration only when actual provider throttling or operational policy supports that preference.

New-worker age can reduce the rate at which assignments are added while the node proves healthy. It should not reduce a displayed machine capacity from 16 to 1 or make existing assignments appear 3,275 percent utilized.

### Operator UI

The fleet page should expose quantities with concrete units:

- assigned mailboxes by protocol;
- active sync sessions and live IMAP sockets;
- running and queued send jobs;
- send successes and attempts over the selected window;
- provider throttles and authentication failures;
- CPU, memory, file descriptors, and network use;
- placement headroom under the configured local resource ceilings.

Recent sends are useful activity telemetry. They are not the denominator for how many idle or lightly active mailboxes a worker can own.

## What the previous Warmbly numbers meant

The `32.75 / 1.00` display was an artifact of two different synthetic calculations:

- `load_score` was the sum of fixed mailbox weights: `1.0` for cold SMTP/IMAP, `0.05` for cold Gmail or Outlook, and `0.4` for any warmup mailbox.
- The materialized view declares a flat base capacity of 16 and multiplies it by a 72-hour worker-age factor in [`000140_worker_decategorization.up.sql`](../internal/infrastructure/db/migrations/000140_worker_decategorization.up.sql).
- `ComputeCapacity` floors that age-adjusted value to 1, but separately calculates a target without the age factor. Placement uses the target, while the admin fleet table displays the age-adjusted effective capacity. The UI can therefore show `/ 1.00` even though placement is scoring against a different denominator.

The `Sends 1H` value is the sum of worker health samples from only the preceding hour. Zero means no recorded attempts in that rolling window, not that the worker cannot send.

The worker detail endpoint defaults to 50 mailboxes and returns cursor pagination in [`internal/repository/pg_admin.go`](../internal/repository/pg_admin.go). The current admin page requests only the first page and does not follow the cursor in [`admin/src/app/dashboard/WorkerDetailPage.tsx`](../admin/src/app/dashboard/WorkerDetailPage.tsx). That is why a worker counted as 88 assignments can show only 50 rows when opened.

## Implementation direction supported by the evidence

1. Remove worker age from the displayed capacity denominator. Keep a short assignment-admission ramp as a placement score or rate, with its own label.
2. Replace the fixed base of 16 and unexplained mailbox-equivalent weights with configurable local ceilings plus live resource measurements.
3. Keep per-mailbox pacing and provider quotas independent of worker assignment capacity.
4. Coordinate tenant and API-project limits across workers.
5. Serialize work per mailbox, bound total concurrent sync/send work per process, and honor provider backoff signals.
6. Treat provider and organization concentration as failure-domain and throttling concerns, not recipient-side worker-IP reputation.
7. Show every assigned mailbox through working cursor pagination or an infinite list.
8. Label the one-hour send metric as activity and provide its window and success/attempt meaning.

The implementation now counts each assigned mailbox once, compares that count with an operator-configured planning target, and keeps provider quotas out of worker capacity. The default target is conservative and configurable because a reliable maximum requires measurement on the actual worker shape and workload.

This model permits dozens or hundreds of low-duty-cycle mailbox accounts on a worker when the machine demonstrates that it has room, while preserving provider limits and deliverability controls in the scopes where they actually apply.
