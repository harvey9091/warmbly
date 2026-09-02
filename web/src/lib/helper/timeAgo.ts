// timeAgo renders a compact relative time ("3m ago", "2d ago"), falling back
// to a plain date once the distance passes a month.
export default function timeAgo(d?: string | Date): string {
    if (!d) return "never";
    const ms = Date.now() - new Date(d).getTime();
    const min = Math.floor(ms / 60_000);
    if (min < 1) return "just now";
    if (min < 60) return `${min}m ago`;
    const h = Math.floor(min / 60);
    if (h < 24) return `${h}h ago`;
    const days = Math.floor(h / 24);
    if (days < 30) return `${days}d ago`;
    return new Date(d).toLocaleDateString();
}
