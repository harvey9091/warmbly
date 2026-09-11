import { Logo } from "@/components/svg";
import useBrand from "@/hooks/useBrand";

/**
 * The wordmark on the sky: sign-in, the CLI authorization page, the cloud
 * connect page.
 *
 * It links to the deployment's website only when there is one. A self-hosted
 * instance that configured no EMAIL_BRAND_WEBSITE_URL renders plain text
 * instead, because the alternative was sending that operator's users to the
 * platform's marketing site from their own login screen.
 */
export default function BrandMark({ className }: { className: string }) {
    const brand = useBrand();
    const inner = (
        <>
            <Logo className="w-7 text-white" />
            <span className="font-extrabold text-[18px] tracking-tight text-white">{brand.name}</span>
        </>
    );
    if (!brand.website_url) return <div className={className}>{inner}</div>;
    return (
        <a href={brand.website_url} className={className}>
            {inner}
        </a>
    );
}
