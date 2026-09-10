// Tells the error backends who is signed in and where they are.
//
// Without this an issue in error tracking is a stack trace and nothing else:
// you can see that a page throws and never which workspace it throws for, or
// what the user did just before. With it, "customer X says campaigns are
// broken" is a search, and the issue itself lists the route they came from.
//
// The identity reaches exception events only, never product analytics, and no
// person profile is created for it. See lib/observability.
import { useEffect } from "react";
import { useLocation } from "react-router-dom";
import { useUserProfile } from "./context/user";
import { useAppStore } from "@/stores";
import { maskIds } from "@/lib/maskIds";
import { noteStep, setErrorIdentity } from "@/lib/observability";

export function ErrorContext() {
    const { user } = useUserProfile();
    const organizationId = useAppStore((s) => s.currentOrganization?.id ?? null);
    const { pathname } = useLocation();

    const userId = user?.id ?? null;

    useEffect(() => {
        setErrorIdentity(userId ? { userId, organizationId } : null);
        // Cleared on unmount, which is what a sign-out is: the next person on
        // a shared machine must not inherit this attribution.
        return () => setErrorIdentity(null);
    }, [userId, organizationId]);

    useEffect(() => {
        noteStep(`Opened ${maskIds(pathname)}`, { path: maskIds(pathname) });
    }, [pathname]);

    return null;
}
