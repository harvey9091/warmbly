import { EyeOffIcon, MonitorIcon, SmartphoneIcon, TabletIcon, type LucideIcon } from "lucide-react";
import {
    deviceKind,
    hiddenReason,
    originSummary,
    type DeviceKind,
    type EngagementKind,
    type OriginLike,
} from "@/lib/engagementOrigin";

const DEVICE_ICON: Record<DeviceKind, LucideIcon | null> = {
    mobile: SmartphoneIcon,
    tablet: TabletIcon,
    desktop: MonitorIcon,
    hidden: EyeOffIcon,
    unknown: null,
};

// The device icon and "iPhone · Apple Mail app" for one open or click.
// Renders nothing when the event said nothing about itself.
export default function OriginBadge({
    origin,
    kind = "open",
    className = "",
}: {
    origin?: OriginLike | null;
    kind?: EngagementKind;
    className?: string;
}) {
    if (!origin) return null;
    const text = originSummary(origin, kind);
    if (!text) return null;
    const Icon = DEVICE_ICON[deviceKind(origin)];
    return (
        <span
            className={`inline-flex items-center gap-1 min-w-0 ${className}`}
            title={hiddenReason(origin) || undefined}
        >
            {Icon && <Icon className="w-3 h-3 shrink-0 text-slate-400" aria-hidden />}
            <span className="truncate">{text}</span>
        </span>
    );
}
