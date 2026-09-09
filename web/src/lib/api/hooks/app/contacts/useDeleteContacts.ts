import deleteContacts from "@/lib/api/client/app/contacts/deleteContacts";
import type ContactSelection from "@/lib/api/models/app/contacts/ContactSelection";
import { useMutation, useQueryClient } from "@tanstack/react-query";

export default function useDeleteContacts() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: (selection: ContactSelection) => deleteContacts(selection),
        onSuccess: (_, selection) => {
            selection.contacts.forEach(id => {
                queryClient.invalidateQueries({
                    queryKey: ["contacts", id]
                });
            });
            // The contacts table reads ["contacts","list",...]; refresh it so the
            // deleted rows disappear without a manual reload. ["campaigns"] carries
            // the lead counts the deleted contacts were part of.
            return Promise.all([
                queryClient.invalidateQueries({ queryKey: ["contacts", "list"] }),
                queryClient.invalidateQueries({ queryKey: ["campaigns"] }),
            ]);
        }
    })
}
