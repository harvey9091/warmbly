import { useQuery } from "@tanstack/react-query";
import getAPIKeyUsageSummary from "@/lib/api/client/app/api-keys/getAPIKeyUsageSummary";

export default function useAPIKeyUsageSummary(enabled = true) {
    return useQuery({
        queryKey: ["api-keys", "usage-summary"],
        queryFn: () => getAPIKeyUsageSummary(),
        enabled,
        refetchInterval: 30_000,
    });
}
