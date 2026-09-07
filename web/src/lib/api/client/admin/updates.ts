// The admin update endpoints, reachable with a dashboard session that holds
// platform admin permissions. They live under /admin, not /v1, so each call
// overrides the client's base URL.

import { API_URL } from "@/lib/information";
import type InstanceUpdate from "../../models/auth/InstanceUpdate";
import type { UpdateJob } from "../../models/auth/InstanceUpdate";
import Request from "../Request";

export async function getInstanceUpdate(withLog = false): Promise<InstanceUpdate> {
    return await Request<InstanceUpdate>({
        method: "GET",
        baseURL: API_URL,
        url: withLog ? "/admin/instance/update?log=1" : "/admin/instance/update",
        authorization: true,
    });
}

export async function checkInstanceUpdate(): Promise<InstanceUpdate> {
    return await Request<InstanceUpdate>({
        method: "POST",
        baseURL: API_URL,
        url: "/admin/instance/update/check",
        authorization: true,
    });
}

export async function applyInstanceUpdate(target = "latest"): Promise<UpdateJob> {
    return await Request<UpdateJob>({
        method: "POST",
        baseURL: API_URL,
        url: "/admin/instance/update/apply",
        data: { target },
        authorization: true,
    });
}
