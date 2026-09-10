// Record ids out of a path.
//
// Admin paths carry them (/dashboard/orgs/<uuid>), and anything reported about
// a page would otherwise ship one, which is also what would turn one broken
// screen into one issue per record. Mirrors web/src/lib/maskIds.ts.
const UUID_SEGMENT = /\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}(?=\/|$)/gi;

export function maskIds(value: string): string {
    return value.replace(UUID_SEGMENT, "/:id");
}
