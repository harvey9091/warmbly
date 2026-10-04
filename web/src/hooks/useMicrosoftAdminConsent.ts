// useMicrosoftAdminConsent: a Global Administrator approves Warmbly for a whole
// Microsoft 365 tenant in a popup; the callback's {state, code} finishes it.

import React from "react";
import { finishMicrosoftGrant, startMicrosoftGrant } from "@/lib/api/client/app/emails/imports/mailboxGrants";
import type { DomainGrant } from "@/lib/api/models/app/emails/MailboxSources";
import useAdminGrantPopup from "@/hooks/useAdminGrantPopup";

export default function useMicrosoftAdminConsent(onGranted?: (grant: DomainGrant) => void, onError?: (message: string, code?: string) => void) {
    const popup = useAdminGrantPopup({
        provider: "Microsoft",
        windowName: "microsoft-admin-consent",
        finish: finishMicrosoftGrant,
        onGranted,
        onError,
    });
    const { open } = popup;
    const start = React.useCallback(() => open(startMicrosoftGrant), [open]);
    return { busy: popup.busy, start, reset: popup.reset };
}
