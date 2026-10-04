// One mark for every mail host, inbox vendor and sign-in provider the app names:
// the Google and Microsoft SVGs, the shipped PNG/SVG logos, else a lettered tile.
import React from "react";
import { ServerIcon } from "lucide-react";
import { Google, Outlook } from "@/components/svg";
import { GENERIC_BRAND_IDS, GOOGLE_BRAND_IDS, MICROSOFT_BRAND_IDS, brandImage, brandLabel, normBrand } from "@/lib/brands";
import { cn } from "@/lib/utils";

type Size = "xs" | "sm" | "md" | "lg" | "xl";

const BOX: Record<Size, string> = {
    xs: "size-4 rounded",
    sm: "size-5 rounded",
    md: "size-6 rounded-md",
    lg: "size-7 rounded-md",
    xl: "size-9 rounded-md",
};
const MARK: Record<Size, string> = {
    xs: "w-2.5 h-2.5",
    sm: "w-3 h-3",
    md: "w-3.5 h-3.5",
    lg: "w-4 h-4",
    xl: "w-5 h-5",
};
// Raster logos carry their own padding, so they sit a touch larger than the SVG marks.
const IMG: Record<Size, string> = {
    xs: "w-3 h-3 rounded-[2px]",
    sm: "w-3.5 h-3.5 rounded-[3px]",
    md: "w-4 h-4 rounded-[3px]",
    lg: "w-[18px] h-[18px] rounded",
    xl: "w-6 h-6 rounded",
};
const LETTER: Record<Size, string> = {
    xs: "text-[8px]",
    sm: "text-[9.5px]",
    md: "text-[10.5px]",
    lg: "text-[11px]",
    xl: "text-[13px]",
};

export default function ProviderLogo({
    id,
    size = "md",
    framed = true,
    muted = false,
    title,
    className,
}: {
    /** A vendor id, a mail_host, a provider ("gmail", "smtp_imap") or "google" / "microsoft". */
    id?: string | null;
    size?: Size;
    /** A bordered white tile around the mark; off for inline use inside a chip. */
    framed?: boolean;
    muted?: boolean;
    title?: string;
    className?: string;
}) {
    const k = normBrand(id);
    const src = brandImage(k);
    const [failed, setFailed] = React.useState(false);
    React.useEffect(() => setFailed(false), [src]);
    const label = brandLabel(k);

    let mark: React.ReactNode;
    if (GOOGLE_BRAND_IDS.has(k)) mark = <Google className={MARK[size]} />;
    else if (MICROSOFT_BRAND_IDS.has(k)) mark = <Outlook className={MARK[size]} />;
    else if (src && !failed)
        mark = (
            <img
                src={src}
                alt=""
                draggable={false}
                loading="lazy"
                onError={() => setFailed(true)}
                className={cn("object-contain", IMG[size])}
            />
        );
    else if (GENERIC_BRAND_IDS.has(k)) mark = <ServerIcon className={cn(MARK[size], "text-slate-400")} />;
    else mark = <span className={cn("font-semibold text-slate-600 leading-none", LETTER[size])}>{label.slice(0, 1).toUpperCase()}</span>;

    return (
        <span
            title={title ?? label}
            className={cn(
                "inline-flex items-center justify-center shrink-0",
                framed && cn("border border-slate-200 bg-white", BOX[size]),
                !framed && BOX[size].split(" ")[0],
                muted && "opacity-60 grayscale",
                className,
            )}
        >
            {mark}
        </span>
    );
}

/** Up to `max` logos overlapping, for a domain on several hosts or a vendor list. */
export function LogoStack({ ids, max = 3, size = "sm", className }: { ids: string[]; max?: number; size?: Size; className?: string }) {
    const unique = [...new Set(ids.map(normBrand).filter(Boolean))];
    if (unique.length === 0) return null;
    const shown = unique.slice(0, max);
    const extra = unique.length - shown.length;
    return (
        <span className={cn("inline-flex items-center", className)} title={unique.map(brandLabel).join(", ")}>
            {shown.map((id, i) => (
                <ProviderLogo key={id} id={id} size={size} title="" className={cn("ring-2 ring-white", i > 0 && "-ml-1.5")} />
            ))}
            {extra > 0 && <span className="ml-1 text-[10.5px] text-slate-400 tabular-nums">+{extra}</span>}
        </span>
    );
}
