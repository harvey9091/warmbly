import { useQuery } from "@tanstack/react-query";
import getInstanceVersion from "../../client/auth/getInstanceVersion";
import type { AppError } from "../../client/normalizeError";

// Feeds the version pill in the header. The backend re-checks releases on its
// own interval, so a five minute poll here is enough for the pill to turn
// amber while the tab is open. A backend without the endpoint (an older
// self-host) answers 404: the pill stays hidden and the poll stops. Any other
// failure keeps polling, because the backend restarting for an update is the
// one moment the pill must come back on its own.
export default function useInstanceVersion() {
    return useQuery({
        queryKey: ["auth", "instance"],
        queryFn: getInstanceVersion,
        staleTime: 5 * 60_000,
        refetchInterval: (q) => ((q.state.error as AppError | null)?.status === 404 ? false : 5 * 60_000),
        retry: false,
    });
}
