import type BulkEditContactsField from "./BulkEditContactsField";
import type ContactSelection from "./ContactSelection";

export default interface BulkEditContacts extends ContactSelection {
    add_campaigns: string[];
    remove_campaigns: string[];
    add_categories?: string[];
    remove_categories?: string[];
    fields: BulkEditContactsField[];
    subscribe?: boolean;
}
