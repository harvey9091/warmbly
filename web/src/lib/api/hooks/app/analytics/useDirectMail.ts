import { useQuery } from "@tanstack/react-query";
import getDirectMail from "@/lib/api/client/app/analytics/getDirectMail";

export default function useDirectMail(period: string = "7d") {
    return useQuery({
        queryKey: ["analytics", "direct", period],
        queryFn: () => getDirectMail(period),
    })
}
