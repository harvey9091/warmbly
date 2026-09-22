import type { ReactElement } from "react";

import { tagMeaning } from "@/lib/unibox/tagMeanings";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export function TagMeaningTooltip({ title, children }: { title: string; children: ReactElement }) {
    const meaning = tagMeaning(title);
    if (!meaning) return children;

    return (
        <Tooltip>
            <TooltipTrigger asChild>{children}</TooltipTrigger>
            <TooltipContent sideOffset={6} className="max-w-72">
                {meaning}
            </TooltipContent>
        </Tooltip>
    );
}
