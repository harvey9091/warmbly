import type InstanceVersion from "../../models/auth/InstanceVersion";
import Request from "../Request";

export default async function getInstanceVersion(): Promise<InstanceVersion> {
    return await Request<InstanceVersion>({
        method: "GET",
        url: "/auth/instance",
        authorization: true,
    });
}
