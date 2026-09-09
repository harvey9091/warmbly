// One machine in the fleet.
//
// Everything shown here is reported BY the node or resolved FOR it. There is
// no install, restart, log or reboot button, because nothing reaches into a
// machine any more: a node enrols with a token, heartbeats, and pulls the
// version it should run. What an operator can actually do is rename it, hold
// it at a version, and forget it.

import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowLeft, Pin, PinOff, RefreshCw, Trash2 } from "lucide-react";
import { PageHeader } from "@/components/layout/PageHeader";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import { getWorkerEmails, getWorkerStats, reassignWorkerEmails } from "@/lib/api/client/admin/workers";
import {
    deleteFleetNode,
    listFleetNodes,
    nodeNeedsUpdate,
    nodeState,
    patchFleetNode,
    type FleetNode,
} from "@/lib/api/client/admin/fleetNodes";
import type { AdminWorkerEmail } from "@/lib/api/models/admin";

const STATE_TONE: Record<string, string> = {
    live: "border-emerald-300 bg-emerald-50 text-emerald-700",
    unreachable: "border-amber-300 bg-amber-50 text-amber-700",
    stopped: "border-zinc-300 text-zinc-600",
};

function Fact({ label, children }: { label: string; children: React.ReactNode }) {
    return (
        <div className="space-y-0.5">
            <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                {label}
            </div>
            <div className="text-[13px]">{children}</div>
        </div>
    );
}

function uptime(seconds?: number): string {
    if (seconds === undefined) return "—";
    const h = Math.floor(seconds / 3600);
    if (h >= 24) return `${Math.floor(h / 24)}d ${h % 24}h`;
    if (h >= 1) return `${h}h ${Math.floor((seconds % 3600) / 60)}m`;
    return `${Math.floor(seconds / 60)}m`;
}

export default function WorkerDetailPage() {
    const { id = "" } = useParams();
    const nav = useNavigate();
    const qc = useQueryClient();

    const [pinDraft, setPinDraft] = useState("");
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [reassignOpen, setReassignOpen] = useState(false);

    const nodeQ = useQuery({
        queryKey: ["admin", "fleet", "nodes"],
        queryFn: () => listFleetNodes(),
        refetchInterval: 30_000,
    });
    const node = (nodeQ.data?.data ?? []).find((n) => n.id === id) ?? null;

    const statsQ = useQuery({
        queryKey: ["admin", "workers", id, "stats"],
        queryFn: () => getWorkerStats(id),
        enabled: !!node && node.role === "worker",
    });

    const emailsQ = useQuery({
        queryKey: ["admin", "workers", id, "emails"],
        queryFn: () => getWorkerEmails(id),
        enabled: !!node && node.role === "worker",
    });

    const patch = useMutation({
        mutationFn: (body: { name?: string; pinned_version?: string }) => patchFleetNode(id, body),
        onSuccess: () => {
            qc.invalidateQueries({ queryKey: ["admin", "fleet"] });
            toast.success("Node updated");
        },
        onError: (e: Error) => toast.error(e.message || "Update failed"),
    });

    const remove = useMutation({
        mutationFn: () => deleteFleetNode(id),
        onSuccess: (res) => {
            toast.success(res.note || "Node removed");
            nav("/workers");
        },
        onError: (e: Error) => toast.error(e.message || "Remove failed"),
    });

    if (nodeQ.isLoading) {
        return <Skeleton className="h-64 w-full" />;
    }
    if (!node) {
        return (
            <div className="space-y-3">
                <PageHeader title="Node not found" description="It may have been removed." />
                <Button asChild size="sm" variant="outline">
                    <Link to="/workers">
                        <ArrowLeft className="size-4" />
                        Fleet
                    </Link>
                </Button>
            </div>
        );
    }

    const state = nodeState(node);
    const mailboxes = emailsQ.data?.data ?? [];

    return (
        <div className="space-y-4">
            <PageHeader title={node.name || node.id.slice(0, 8)} description={`${node.role} node`}>
                <div className="flex gap-2">
                    <Button asChild size="sm" variant="outline">
                        <Link to="/workers">
                            <ArrowLeft className="size-4" />
                            Fleet
                        </Link>
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => nodeQ.refetch()}>
                        <RefreshCw className="size-4" />
                        Refresh
                    </Button>
                </div>
            </PageHeader>

            <Card>
                <CardHeader>
                    <CardTitle className="text-base">Machine</CardTitle>
                    <CardDescription>
                        Reported by the node on its last heartbeat.
                    </CardDescription>
                </CardHeader>
                <CardContent className="grid grid-cols-2 gap-4 md:grid-cols-4">
                    <Fact label="State">
                        <Badge variant="outline" className={`text-[10px] ${STATE_TONE[state]}`}>
                            {state}
                        </Badge>
                    </Fact>
                    <Fact label="Version">
                        {nodeNeedsUpdate(node) ? (
                            <span className="font-mono text-xs">
                                {node.version || "—"}
                                <span className="text-muted-foreground"> → </span>
                                <span className="text-amber-600">{node.desired_version}</span>
                            </span>
                        ) : (
                            <span className="font-mono text-xs">{node.version || "—"}</span>
                        )}
                    </Fact>
                    <Fact label="Address">
                        <span className="font-mono text-xs">{node.address || "—"}</span>
                    </Fact>
                    <Fact label="Region">
                        <span className="font-mono text-xs">{node.region || "—"}</span>
                    </Fact>
                    <Fact label="Memory">
                        {node.usage?.memory_mb !== undefined ? `${node.usage.memory_mb} MB` : "—"}
                    </Fact>
                    <Fact label="Goroutines">{node.usage?.goroutines ?? "—"}</Fact>
                    <Fact label="Uptime">{uptime(node.usage?.uptime_seconds)}</Fact>
                    <Fact label="Last seen">
                        {node.last_seen_at ? new Date(node.last_seen_at).toLocaleString() : "never"}
                    </Fact>
                    <Fact label="Enrolled">{new Date(node.enrolled_at).toLocaleString()}</Fact>
                    <Fact label="Node id">
                        <span className="font-mono text-[11px]">{node.id}</span>
                    </Fact>
                    {node.last_error && (
                        <div className="col-span-2 md:col-span-4">
                            <Fact label="Last error">
                                <span className="text-red-600">{node.last_error}</span>
                            </Fact>
                        </div>
                    )}
                </CardContent>
            </Card>

            <Card>
                <CardHeader>
                    <CardTitle className="text-base">Version</CardTitle>
                    <CardDescription>
                        The node pulls whatever the fleet is set to. Pin it to hold this one
                        machine back, or to canary a release on it before the rest follow.
                    </CardDescription>
                </CardHeader>
                <CardContent className="flex flex-wrap items-end gap-2">
                    <div className="space-y-1.5">
                        <Input
                            value={pinDraft}
                            onChange={(e) => setPinDraft(e.target.value)}
                            placeholder={node.pinned_version || "v1.4.2"}
                            className="w-48"
                        />
                    </div>
                    <Button
                        size="sm"
                        disabled={!pinDraft.trim() || patch.isPending}
                        onClick={() => {
                            patch.mutate({ pinned_version: pinDraft.trim() });
                            setPinDraft("");
                        }}
                    >
                        <Pin className="size-4" />
                        Pin
                    </Button>
                    {node.pinned_version && (
                        <Button
                            size="sm"
                            variant="outline"
                            disabled={patch.isPending}
                            onClick={() => patch.mutate({ pinned_version: "" })}
                        >
                            <PinOff className="size-4" />
                            Clear pin ({node.pinned_version})
                        </Button>
                    )}
                </CardContent>
            </Card>

            {node.role === "worker" && (
                <Card>
                    <CardHeader>
                        <CardTitle className="text-base">
                            Mailboxes {mailboxes.length > 0 && `(${mailboxes.length})`}
                        </CardTitle>
                        <CardDescription>
                            Placement assigns these; you never have to. Moving one by hand is
                            temporary — the rotation loop re-places it if it disagrees.
                        </CardDescription>
                    </CardHeader>
                    <CardContent className="space-y-3">
                        {statsQ.data && (
                            <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
                                <Fact label="Sent today">{statsQ.data.emails_sent_today}</Fact>
                                <Fact label="Sent this week">{statsQ.data.emails_sent_this_week}</Fact>
                                <Fact label="Sent total">{statsQ.data.total_emails_sent}</Fact>
                                {/* Already a percentage in SQL; multiplying again gives 10000%. */}
                                <Fact label="Success rate">
                                    {`${Math.round(statsQ.data.success_rate)}%`}
                                </Fact>
                            </div>
                        )}

                        {emailsQ.isLoading ? (
                            <Skeleton className="h-24 w-full" />
                        ) : mailboxes.length === 0 ? (
                            <p className="text-[13px] text-muted-foreground">
                                No mailboxes on this worker yet.
                            </p>
                        ) : (
                            <div className="rounded-md border">
                                {mailboxes.map((m: AdminWorkerEmail) => (
                                    <label
                                        key={m.id}
                                        className="flex items-center gap-2 border-b px-3 py-2 text-[12.5px] last:border-b-0"
                                    >
                                        <Checkbox
                                            checked={selected.has(m.id)}
                                            onCheckedChange={(v) => {
                                                const next = new Set(selected);
                                                if (v) next.add(m.id);
                                                else next.delete(m.id);
                                                setSelected(next);
                                            }}
                                        />
                                        <span className="font-mono">{m.email}</span>
                                        <Badge variant="outline" className="ml-auto text-[10px]">
                                            {m.provider}
                                        </Badge>
                                    </label>
                                ))}
                            </div>
                        )}

                        {selected.size > 0 && (
                            <Button size="sm" onClick={() => setReassignOpen(true)}>
                                Move {selected.size} elsewhere
                            </Button>
                        )}
                    </CardContent>
                </Card>
            )}

            <Card>
                <CardHeader>
                    <CardTitle className="text-base">Remove</CardTitle>
                    <CardDescription>
                        Forgets the node. Any mailboxes it carries are re-placed within a few
                        minutes. It does not stop the process: a machine that is still running
                        re-joins on its next heartbeat, so stop the service there too.
                    </CardDescription>
                </CardHeader>
                <CardContent>
                    <Button
                        size="sm"
                        variant="destructive"
                        disabled={remove.isPending}
                        onClick={() => remove.mutate()}
                    >
                        <Trash2 className="size-4" />
                        {remove.isPending ? "Removing…" : "Remove from fleet"}
                    </Button>
                </CardContent>
            </Card>

            <ReassignDialog
                open={reassignOpen}
                onOpenChange={setReassignOpen}
                source={node}
                mailboxIds={[...selected]}
                onDone={() => {
                    setSelected(new Set());
                    emailsQ.refetch();
                    nodeQ.refetch();
                }}
            />
        </div>
    );
}

function ReassignDialog({
    open,
    onOpenChange,
    source,
    mailboxIds,
    onDone,
}: {
    open: boolean;
    onOpenChange: (v: boolean) => void;
    source: FleetNode;
    mailboxIds: string[];
    onDone: () => void;
}) {
    const [target, setTarget] = useState("");

    const workersQ = useQuery({
        queryKey: ["admin", "fleet", "nodes", "worker"],
        queryFn: () => listFleetNodes("worker"),
        enabled: open,
        staleTime: 30_000,
    });
    const candidates = (workersQ.data?.data ?? []).filter((x) => x.id !== source.id);
    const chosen = candidates.find((x) => x.id === target) ?? null;

    const mutation = useMutation({
        mutationFn: () => reassignWorkerEmails(target, mailboxIds),
        onSuccess: () => {
            toast.success(
                `${mailboxIds.length} mailbox${mailboxIds.length === 1 ? "" : "es"} moved`,
            );
            setTarget("");
            onDone();
            onOpenChange(false);
        },
        onError: (e: Error) => toast.error(e.message || "Reassign failed"),
    });

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent>
                <DialogHeader>
                    <DialogTitle>
                        Move {mailboxIds.length} mailbox{mailboxIds.length === 1 ? "" : "es"}
                    </DialogTitle>
                    <DialogDescription>
                        Sending and sync continue from the target on its next heartbeat. Any
                        worker can host any mailbox, so this is only worth doing when you know
                        something placement does not.
                    </DialogDescription>
                </DialogHeader>

                <Select value={target || undefined} onValueChange={setTarget}>
                    <SelectTrigger className="h-8 w-full text-[12.5px]">
                        <SelectValue
                            placeholder={workersQ.isLoading ? "Loading…" : "Pick a worker"}
                        />
                    </SelectTrigger>
                    <SelectContent>
                        {candidates.length === 0 && (
                            <div className="px-2 py-1.5 text-xs text-muted-foreground">
                                No other workers.
                            </div>
                        )}
                        {candidates.map((x) => (
                            <SelectItem key={x.id} value={x.id} className="text-[12.5px]">
                                {x.name || x.id.slice(0, 8)}
                                {x.region ? ` · ${x.region}` : ""} · {nodeState(x)} ·{" "}
                                {x.mailbox_count ?? 0} mailbox
                                {(x.mailbox_count ?? 0) === 1 ? "" : "es"}
                            </SelectItem>
                        ))}
                    </SelectContent>
                </Select>

                {chosen && nodeState(chosen) !== "live" && (
                    <p className="text-[11px] text-amber-700">
                        That node is {nodeState(chosen)}; placement would not choose it, and the
                        rotation loop will move these mailboxes off it again.
                    </p>
                )}

                <DialogFooter>
                    <Button variant="outline" onClick={() => onOpenChange(false)}>
                        Cancel
                    </Button>
                    <Button
                        onClick={() => mutation.mutate()}
                        disabled={!chosen || mailboxIds.length === 0 || mutation.isPending}
                    >
                        {mutation.isPending ? "Moving…" : "Move"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
