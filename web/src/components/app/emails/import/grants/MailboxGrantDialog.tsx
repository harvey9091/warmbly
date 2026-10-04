// MailboxGrantDialog: the admin grant view opened from a mailbox connected
// through one, with that grant picked: its status, Check again, removal, and
// its directory for connecting more mailboxes. The sign-in migration opens it
// on the setup of a new domain instead.
import ProviderLogo from "@/components/app/emails/ProviderLogo";
import { GRANT_PROVIDER_LABELS, type GrantProvider } from "@/lib/api/models/app/emails/MailboxSources";
import ImportDialogShell from "../ImportDialogShell";
import GrantImportWizard from "./GrantImportWizard";

export default function MailboxGrantDialog({
    open,
    provider,
    grantId,
    initialDomain,
    onClose,
}: {
    open: boolean;
    provider: GrantProvider;
    grantId?: string;
    /** Opens the setup of a new domain with this one filled in. */
    initialDomain?: string;
    onClose: () => void;
}) {
    return (
        <ImportDialogShell
            open={open}
            title={`${GRANT_PROVIDER_LABELS[provider]} admin grant`}
            icon={<ProviderLogo id={provider} size="xs" framed={false} />}
            onClose={onClose}
            discardText="Discard what you picked and entered here?"
        >
            {({ onAllowance, setDirty }) => (
                <GrantImportWizard
                    key={grantId ?? initialDomain ?? provider}
                    provider={provider}
                    initialGrantId={grantId}
                    initialDomain={initialDomain}
                    onDone={onClose}
                    onAllowance={onAllowance}
                    onDirtyChange={setDirty}
                />
            )}
        </ImportDialogShell>
    );
}
