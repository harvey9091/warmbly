import React from "react";
import {
    PopoverMenu,
    PopoverMenuContent,
    PopoverMenuItem,
    PopoverMenuTrigger,
    SelectButton,
} from "@/components/ui/popover-menu";
import useCampaigns from "@/lib/api/hooks/app/campaigns/useCampaigns";

// CampaignPicker — house-theme PopoverMenu campaign selector backed by the
// existing campaigns list query. Single-select with an explicit "None". Shared
// by the sheet sync wizard and the automation builder's campaign actions.
export default function CampaignPicker({
    campaignId,
    campaignName,
    onChange,
    noneLabel = "No campaign",
    className = "w-full",
}: {
    campaignId: string | null;
    campaignName: string;
    onChange: (id: string | null, name: string) => void;
    noneLabel?: string;
    className?: string;
}) {
    const [query, setQuery] = React.useState("");
    const campaigns = useCampaigns({ query, folder: "" });
    // A saved id whose name is not cached yet (an automation reopened later)
    // resolves from the list once it loads.
    const resolved = campaignId ? campaigns.campaigns.find((c) => c.id === campaignId)?.name : undefined;
    const label = campaignId ? campaignName || resolved || "Selected campaign" : noneLabel;

    return (
        <PopoverMenu align="start">
            <PopoverMenuTrigger asChild>
                <SelectButton label={label} className={className} />
            </PopoverMenuTrigger>
            <PopoverMenuContent minWidth={280}>
                <div className="px-2 py-1.5 border-b border-slate-200">
                    <input
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                        placeholder="Search campaigns…"
                        className="w-full h-5 bg-transparent text-[12px] text-slate-900 placeholder:text-slate-400 outline-none"
                    />
                </div>
                <PopoverMenuItem selected={!campaignId} onSelect={() => onChange(null, "")}>
                    {noneLabel}
                </PopoverMenuItem>
                {campaigns.campaigns.map((c) => (
                    <PopoverMenuItem
                        key={c.id}
                        selected={c.id === campaignId}
                        onSelect={() => onChange(c.id, c.name)}
                    >
                        {c.name}
                    </PopoverMenuItem>
                ))}
                {campaigns.campaigns.length === 0 && (
                    <div className="px-3 py-2 text-[11.5px] text-slate-400 text-center">
                        {campaigns.isPending ? "Loading…" : "No campaigns found."}
                    </div>
                )}
            </PopoverMenuContent>
        </PopoverMenu>
    );
}
