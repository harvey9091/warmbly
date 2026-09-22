import { useMutation } from "@tanstack/react-query";
import linkSSO from "../../client/auth/linkSSO";

export default function useLinkSSO() {
    return useMutation({
        mutationFn: ({ pending_token, password }: { pending_token: string; password: string }) =>
            linkSSO(pending_token, password),
    });
}
