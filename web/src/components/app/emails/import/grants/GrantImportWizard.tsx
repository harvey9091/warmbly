// GrantImportWizard: the whole-domain (Google) and whole-organization
// (Microsoft) routes of the connect modal, and the grant view the mailbox
// drawer and the sign-in migration open. A new grant is a guided checklist
// (Google: Authorize -> Verify, Microsoft: Admin approval); a workspace that
// already holds grants starts at the list of them. Then Mailboxes (the granted
// directory) -> Settings -> Import, the last being the file import's result screen.
import React from "react";
import { useConnectGrantUsers, useGrantConfig, useGrantUsers, useGrants } from "@/lib/api/hooks/app/emails/useMailboxGrants";
import { domainOf, grantName, type DirectoryUser, type DomainGrant, type GrantProvider } from "@/lib/api/models/app/emails/MailboxSources";
import useGoogleAdminSignin from "@/hooks/useGoogleAdminSignin";
import useMicrosoftAdminConsent from "@/hooks/useMicrosoftAdminConsent";
import type { AppError } from "@/lib/api/client/normalizeError";
import buildError from "@/lib/helper/buildError";
import type { PickItem } from "../PickTable";
import SourceImportWizard, { type SetupStep } from "../SourceImportWizard";
import GrantAdminStep from "./GrantAdminStep";
import { GoogleAuthorizeStep, GoogleVerifyStep, MicrosoftConsentStep } from "./GrantSetupSteps";
import { useGoogleDraft } from "./googleDraft";

function toItem(u: DirectoryUser): PickItem {
    return {
        id: u.id,
        email: u.email,
        name: u.name,
        domain: domainOf(u.email),
        status: u.enabled ? undefined : { label: "Suspended", cls: "text-amber-700 bg-amber-50" },
        connected: u.connected,
        upgrade: u.upgrade === true,
        disabledReason: u.enabled ? undefined : "Suspended or disabled in the directory, so it cannot sign in.",
    };
}

export default function GrantImportWizard({
    provider,
    initialGrantId,
    initialDomain,
    onDone,
    onAllowance,
    onDirtyChange,
}: {
    provider: GrantProvider;
    /** Opens with this grant picked, e.g. from a mailbox connected through it. */
    initialGrantId?: string;
    /** Google: opens the setup of a new domain with this one filled in. */
    initialDomain?: string;
    onDone: () => void;
    onAllowance?: () => void;
    onDirtyChange?: (dirty: boolean) => void;
}) {
    const config = useGrantConfig();
    const list = useGrants();
    const [grantId, setGrantId] = React.useState<string | null>(initialGrantId ?? null);
    const users = useGrantUsers(grantId);
    const connect = useConnectGrantUsers();
    const goRef = React.useRef<((key: string) => void) | null>(null);

    const grants = React.useMemo(() => (list.data?.data ?? []).filter((g) => g.provider === provider), [list.data, provider]);
    const grant = grants.find((g) => g.id === grantId) ?? null;
    const directory = users.data?.data;
    const items = React.useMemo(() => directory?.map(toItem), [directory]);

    const enabled = provider === "google" ? config.data?.google_enabled === true : config.data?.microsoft_enabled === true;
    const loading = list.isLoading || config.isLoading;
    // A new grant is set up step by step; with grants already here the list comes first.
    const [adding, setAdding] = React.useState(!!initialDomain);
    const noGrants = enabled && !loading && grants.length === 0;
    React.useEffect(() => {
        if (noGrants) setAdding(true);
    }, [noGrants]);
    const setupMode = enabled && adding;
    const cancelSetup = grants.length > 0 ? () => setAdding(false) : undefined;

    const draft = useGoogleDraft(initialDomain ?? "");
    const [msError, setMsError] = React.useState<string | null>(null);

    const granted = React.useCallback((g: DomainGrant) => {
        setGrantId(g.id);
        if (g.status === "active") goRef.current?.("pick");
    }, []);
    const googleSignin = useGoogleAdminSignin(
        (g) => {
            draft.reset();
            granted(g);
        },
        (text, code) => draft.setError({ text, code }),
    );
    const msConsent = useMicrosoftAdminConsent((g) => {
        setMsError(null);
        granted(g);
    }, setMsError);

    const noun = provider === "google" ? "domain" : "organization";
    const grantIssue = !grant
        ? null
        : grant.status === "invalid"
          ? "This grant failed its last check. Check it again first."
          : null;

    let setupSteps: SetupStep[];
    if (setupMode && provider === "google") {
        setupSteps = [
            {
                key: "authorize",
                label: "Authorize",
                issue: null,
                cta: "I've authorized it",
                render: () => <GoogleAuthorizeStep config={config.data} onCancel={cancelSetup} />,
            },
            {
                key: "verify",
                label: "Verify domain",
                issue: grant ? grantIssue : "Prove the domain is yours first: sign in as its super admin, or check the DNS record.",
                render: (_next, goTo) => (
                    <GoogleVerifyStep
                        draft={draft}
                        signin={googleSignin}
                        onGranted={(g) => {
                            draft.reset();
                            granted(g);
                        }}
                        onBackToAuthorize={() => goTo("authorize")}
                    />
                ),
            },
        ];
    } else if (setupMode) {
        setupSteps = [
            {
                key: "consent",
                label: "Admin approval",
                issue: grant ? grantIssue : "A Global Administrator has to approve Warmbly first.",
                render: () => <MicrosoftConsentStep consent={msConsent} error={msError} granted={grant} onCancel={cancelSetup} />,
            },
        ];
    } else {
        setupSteps = [
            {
                key: "source",
                label: provider === "google" ? "Domain" : "Organization",
                issue: grant ? grantIssue : `Connect a ${noun} first, or pick one.`,
                render: (next) => (
                    <GrantAdminStep
                        provider={provider}
                        config={config.data}
                        grants={grants}
                        loading={loading}
                        selectedId={grantId}
                        onSelect={(g, advance) => {
                            setGrantId(g?.id ?? null);
                            if (g && advance) next();
                        }}
                        onAddNew={() => {
                            setGrantId(null);
                            setAdding(true);
                        }}
                    />
                ),
            },
        ];
    }

    return (
        <SourceImportWizard
            setupSteps={setupSteps}
            goRef={goRef}
            onStartOver={() => {
                if (grants.length > 0) setAdding(false);
            }}
            sourceKey={grantId}
            sourceDirty={provider === "google" && setupMode && !grant && draft.dirty}
            pickNote={() =>
                grant ? (
                    <p className="text-[11.5px] text-slate-500">
                        Accounts on <span className="text-slate-700 font-medium">{grantName(grant)}</span>. They connect through the
                        grant: no passwords, and nothing for each person to approve.
                        {items?.some((i) => i.upgrade) &&
                            " A mailbox marked Moves here signs in on its own today; it keeps its history, campaigns and warmup when it moves."}
                    </p>
                ) : null
            }
            items={items}
            itemsLoading={!!grantId && users.isLoading}
            itemsLoadingLogo={provider}
            itemsLoadingSteps={
                grant
                    ? [
                          provider === "google" ? "Asking Google for your directory…" : "Asking Microsoft for your organization's users…",
                          `Listing everyone on ${grantName(grant)}…`,
                          "Checking which are already in this workspace…",
                      ]
                    : undefined
            }
            itemsFetching={users.isFetching}
            itemsError={users.error ? buildError(users.error as unknown as AppError) : null}
            onRefresh={() => void users.refetch()}
            source="grant"
            submit={(ids, options) => connect.mutateAsync({ id: grantId!, body: { user_ids: ids, options } })}
            submitting={connect.isPending}
            anotherLabel="Connect more mailboxes"
            onDone={onDone}
            onAllowance={onAllowance}
            onDirtyChange={onDirtyChange}
        />
    );
}
