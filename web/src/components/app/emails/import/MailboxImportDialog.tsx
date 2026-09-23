// MailboxImportDialog: reopens a running or recent import at its result
// screen, from the Imports menu on the mailboxes page.
import { FileSpreadsheetIcon } from "lucide-react";
import ImportDialogShell from "./ImportDialogShell";
import MailboxImportWizard from "./MailboxImportWizard";

export default function MailboxImportDialog({ importId, onClose }: { importId: string | null; onClose: () => void }) {
    return (
        <ImportDialogShell open={!!importId} title="Import mailboxes" icon={<FileSpreadsheetIcon className="w-3 h-3" />} onClose={onClose}>
            {({ onAllowance, setDirty }) => (
                // "Import another list" inside the wizard leaves the result screen with a draft to guard.
                <MailboxImportWizard
                    key={importId}
                    importId={importId ?? undefined}
                    onDone={onClose}
                    onAllowance={onAllowance}
                    onDirtyChange={setDirty}
                />
            )}
        </ImportDialogShell>
    );
}
