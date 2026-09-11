// Record ids out of a path.
//
// Dashboard paths carry them (/app/campaigns/<uuid>), and anything reported
// about a page would otherwise ship one. They are not personal data, but they
// are identifiers, and the point of the cookieless setup is that no such value
// exists to join on. Masking them is also what keeps a report readable: one row
// per screen instead of one row per record.
const UUID_SEGMENT = /\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}(?=\/|$)/gi;

export function maskIds(value: string): string {
    return value.replace(UUID_SEGMENT, "/:id");
}
