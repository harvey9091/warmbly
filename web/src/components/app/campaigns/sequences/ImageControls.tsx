// Image insertion and editing for the campaign body editor (issue #380).
//
// Two surfaces: a toolbar menu that uploads, takes a URL, or picks from the
// workspace library, and a bubble over the selected image for size, alignment
// and alt text. Uploads go to the library because a body image is fetched by
// the recipient's mail client, which has no session and cannot read a
// presigned attachment URL.

import React from "react";
import { createPortal } from "react-dom";
import { AnimatePresence, motion } from "framer-motion";
import {
    AlignCenterIcon,
    AlignLeftIcon,
    AlignRightIcon,
    ImageIcon,
    Loader2Icon,
    Trash2Icon,
    UploadCloudIcon,
} from "lucide-react";
import toast from "react-hot-toast";
import type { Editor } from "@tiptap/react";
import { NodeSelection } from "@tiptap/pm/state";
import useClickOutside from "@/hooks/useClickOutside";
import { useAnchoredFloating } from "@/hooks/useAnchoredFloating";
import { useConfirm } from "@/hooks/context/confirm";
import { useEmailImages, useDeleteEmailImage } from "@/lib/api/hooks/app/campaigns/useEmailImages";
import type EmailImage from "@/lib/api/models/app/campaigns/EmailImage";
import formatBytes from "@/lib/helper/formatBytes";
import { ACCEPTED_IMAGE_TYPES, insertImage, useImageUpload } from "./imageUpload";
import { IMAGE_SIZE_PRESETS, type ImageAlign } from "./nodes/EmailImageNode";

export function ImageMenu({ editor }: { editor: Editor }) {
    const [open, setOpen] = React.useState(false);
    const [url, setUrl] = React.useState("");
    const [dragging, setDragging] = React.useState(false);
    const ref = React.useRef<HTMLDivElement>(null);
    const fileRef = React.useRef<HTMLInputElement>(null);
    useClickOutside(ref, () => setOpen(false));
    const { setReference, setFloating, floatingStyle } = useAnchoredFloating(open, {
        placement: "bottom-start",
        gap: 6,
        maxHeight: true,
    });

    // The library only loads while the menu is open: most steps never insert an
    // image and the list is a per-workspace read.
    const { data, isLoading, fetchNextPage, hasNextPage, isFetchingNextPage } = useEmailImages(open);
    const images = React.useMemo(() => (data?.pages ?? []).flatMap((p) => p.data), [data]);
    const { run: upload, isUploading } = useImageUpload();
    const del = useDeleteEmailImage();
    const confirm = useConfirm();

    React.useEffect(() => {
        if (!open) return;
        const onKey = (e: KeyboardEvent) => {
            if (e.key !== "Escape") return;
            // The delete confirmation sits above this menu, so Escape belongs
            // to it first: only the innermost layer closes.
            if (document.querySelector("[role='alertdialog']")) return;
            e.stopPropagation();
            setOpen(false);
        };
        document.addEventListener("keydown", onKey, true);
        return () => document.removeEventListener("keydown", onKey, true);
    }, [open]);

    const pick = async (files: FileList | File[] | null) => {
        const file = Array.from(files ?? [])[0];
        if (!file) return;
        const created = await upload(file);
        if (!created) return;
        insertImage(editor, { url: created.url, alt: created.filename });
        setOpen(false);
    };

    const applyUrl = () => {
        const u = url.trim();
        // https only: the dashboard is served over TLS, so a http:// image is
        // blocked as mixed content in the preview the author is looking at.
        if (!/^https:\/\//i.test(u)) {
            toast.error("Enter a full https:// image address.");
            return;
        }
        insertImage(editor, { url: u });
        setUrl("");
        setOpen(false);
    };

    const remove = (img: EmailImage) => {
        confirm.show(
            `Delete "${img.filename}" from the library? Emails already sent with it lose the image.`,
            async () => {
                await del.mutateAsync(img.id);
                toast.success("Image deleted.");
            },
        );
    };

    return (
        <div ref={ref} className="relative">
            <button
                ref={(el) => setReference(el)}
                type="button"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => setOpen((o) => !o)}
                title="Insert an image"
                aria-pressed={open}
                className={`size-7 inline-flex items-center justify-center rounded transition-colors ${
                    open ? "bg-sky-50 text-sky-700" : "text-slate-500 hover:text-slate-900 hover:bg-slate-100"
                }`}
            >
                <ImageIcon className="w-3.5 h-3.5" />
            </button>
            {typeof document !== "undefined" &&
                createPortal(
                    <AnimatePresence>
                        {open && (
                            <motion.div
                                ref={setFloating}
                                data-floating=""
                                style={floatingStyle}
                                initial={{ opacity: 0 }}
                                animate={{ opacity: 1 }}
                                exit={{ opacity: 0 }}
                                transition={{ duration: 0.12 }}
                                className="z-[60] w-[340px] max-w-[calc(100vw-24px)] overflow-y-auto rounded-md border border-slate-200 bg-white shadow-[0_12px_32px_-8px_rgba(15,23,42,0.18)]"
                            >
                                <div className="border-b border-slate-100 px-3 py-2">
                                    <p className="text-[12px] font-medium text-slate-800">Insert an image</p>
                                    <p className="mt-0.5 text-[10.5px] text-slate-400">
                                        Cold email lands better with few images. One signature logo or product shot is
                                        plenty.
                                    </p>
                                </div>

                                <div className="p-2">
                                    <button
                                        type="button"
                                        onMouseDown={(e) => e.preventDefault()}
                                        onClick={() => fileRef.current?.click()}
                                        onDragOver={(e) => {
                                            e.preventDefault();
                                            setDragging(true);
                                        }}
                                        onDragLeave={() => setDragging(false)}
                                        onDrop={(e) => {
                                            e.preventDefault();
                                            setDragging(false);
                                            void pick(e.dataTransfer?.files ?? null);
                                        }}
                                        disabled={isUploading}
                                        className={`flex w-full flex-col items-center justify-center gap-1 rounded-md border border-dashed px-3 py-4 transition-colors ${
                                            dragging
                                                ? "border-sky-400 bg-sky-50/60"
                                                : "border-slate-200 hover:border-sky-300 hover:bg-sky-50/40"
                                        } disabled:opacity-60`}
                                    >
                                        {isUploading ? (
                                            <Loader2Icon className="w-4 h-4 animate-spin text-sky-600" />
                                        ) : (
                                            <UploadCloudIcon className="w-4 h-4 text-slate-400" />
                                        )}
                                        <span className="text-[11.5px] text-slate-600">
                                            {isUploading ? "Uploading…" : "Drop an image, or click to choose"}
                                        </span>
                                        <span className="text-[10px] text-slate-400">PNG, JPG, GIF or WebP · up to 5 MB</span>
                                    </button>
                                    <input
                                        ref={fileRef}
                                        type="file"
                                        accept={ACCEPTED_IMAGE_TYPES}
                                        className="hidden"
                                        onChange={(e) => {
                                            void pick(e.target.files);
                                            e.target.value = "";
                                        }}
                                    />
                                </div>

                                <div className="px-2 pb-2">
                                    <div className="px-1 pb-1 text-[10px] uppercase tracking-[0.14em] text-slate-400">
                                        By address
                                    </div>
                                    <div className="flex items-center gap-1.5">
                                        <input
                                            value={url}
                                            onChange={(e) => setUrl(e.target.value)}
                                            onKeyDown={(e) => {
                                                if (e.key === "Enter") {
                                                    e.preventDefault();
                                                    applyUrl();
                                                }
                                            }}
                                            placeholder="https://…/logo.png"
                                            className="h-7 min-w-0 flex-1 rounded-md border border-slate-200 bg-white px-2 text-[12px] text-slate-900 placeholder:text-slate-400 outline-none focus:border-sky-400 focus:ring-2 focus:ring-sky-100"
                                        />
                                        <button
                                            type="button"
                                            onMouseDown={(e) => e.preventDefault()}
                                            onClick={applyUrl}
                                            disabled={!url.trim()}
                                            className="h-7 shrink-0 rounded-md bg-sky-600 px-2.5 text-[11.5px] font-medium text-white transition-colors hover:bg-sky-700 disabled:opacity-50"
                                        >
                                            Insert
                                        </button>
                                    </div>
                                </div>

                                <div className="px-2 pb-2">
                                    <div className="px-1 pb-1 text-[10px] uppercase tracking-[0.14em] text-slate-400">
                                        Workspace library
                                    </div>
                                    {isLoading ? (
                                        <div className="px-1 py-2 text-[11.5px] text-slate-400">Loading…</div>
                                    ) : images.length === 0 ? (
                                        <div className="px-1 py-2 text-[11.5px] text-slate-400">
                                            Nothing uploaded yet. Images you add here are reusable across every campaign.
                                        </div>
                                    ) : (
                                        <div className="grid max-h-56 grid-cols-3 gap-1.5 overflow-y-auto">
                                            {images.map((img) => (
                                                <div key={img.id} className="group relative">
                                                    <button
                                                        type="button"
                                                        title={`${img.filename} · ${formatBytes(img.size)}`}
                                                        onMouseDown={(e) => e.preventDefault()}
                                                        onClick={() => {
                                                            insertImage(editor, { url: img.url, alt: img.filename });
                                                            setOpen(false);
                                                        }}
                                                        className="block aspect-square w-full overflow-hidden rounded-md border border-slate-200 bg-slate-50 transition-colors hover:border-sky-300"
                                                    >
                                                        <img
                                                            src={img.url}
                                                            alt={img.filename}
                                                            loading="lazy"
                                                            className="h-full w-full object-contain"
                                                        />
                                                    </button>
                                                    <button
                                                        type="button"
                                                        title="Delete from the library"
                                                        onMouseDown={(e) => e.preventDefault()}
                                                        onClick={() => remove(img)}
                                                        className="absolute right-0.5 top-0.5 size-5 inline-flex items-center justify-center rounded bg-white/90 text-slate-400 opacity-100 transition-colors hover:text-rose-600 md:opacity-0 md:group-hover:opacity-100"
                                                    >
                                                        <Trash2Icon className="w-3 h-3" />
                                                    </button>
                                                </div>
                                            ))}
                                        </div>
                                    )}
                                    {hasNextPage && (
                                        <button
                                            type="button"
                                            onMouseDown={(e) => e.preventDefault()}
                                            onClick={() => void fetchNextPage()}
                                            disabled={isFetchingNextPage}
                                            className="mt-1.5 h-6 w-full rounded text-[11.5px] font-medium text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900 disabled:opacity-50"
                                        >
                                            {isFetchingNextPage ? "Loading…" : "Show older"}
                                        </button>
                                    )}
                                </div>
                            </motion.div>
                        )}
                    </AnimatePresence>,
                    document.body,
                )}
        </div>
    );
}

// selectedImage returns the image the caret has selected as a node, or null.
// The bubble only exists for that selection, so clicking away dismisses it
// without a listener of its own.
function selectedImage(editor: Editor): { pos: number; attrs: Record<string, unknown> } | null {
    const sel = editor.state.selection;
    if (!(sel instanceof NodeSelection) || sel.node.type.name !== "image") return null;
    return { pos: sel.from, attrs: sel.node.attrs };
}

// The editor re-renders its host on every transaction (shouldRerenderOnTransaction),
// so this reads the live selection on each render rather than subscribing again.
export function ImageBubble({ editor }: { editor: Editor }) {
    const [anchor, setAnchor] = React.useState<{ top: number; left: number } | null>(null);

    const selection = selectedImage(editor);
    const selectedPos = selection?.pos ?? null;

    // Follow the image through scrolling and resizes, the same way the AI pill
    // does, so the bubble never detaches from what it edits.
    React.useEffect(() => {
        if (selectedPos === null) {
            setAnchor(null);
            return;
        }
        const sync = () => {
            try {
                const box = editor.view.coordsAtPos(selectedPos);
                setAnchor({ top: box.top, left: box.left });
            } catch {
                setAnchor(null);
            }
        };
        sync();
        window.addEventListener("scroll", sync, true);
        window.addEventListener("resize", sync);
        return () => {
            window.removeEventListener("scroll", sync, true);
            window.removeEventListener("resize", sync);
        };
    }, [selectedPos, editor]);

    if (typeof document === "undefined" || !selection || !anchor) return null;

    const align = (selection.attrs.align as ImageAlign) ?? "left";
    const width = (selection.attrs.width as number | null) ?? null;
    const alt = (selection.attrs.alt as string | null) ?? "";
    // No focus() here on purpose: the alt-text field is part of this bar, and
    // pulling focus back into the editor on every keystroke would make it
    // impossible to type in. ProseMirror keeps the node selected regardless.
    const set = (attrs: Record<string, unknown>) => editor.commands.updateAttributes("image", attrs);

    const alignBtn = (value: ImageAlign, Icon: typeof AlignLeftIcon, title: string) => (
        <button
            type="button"
            title={title}
            aria-pressed={align === value}
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => set({ align: value })}
            className={`size-6 inline-flex items-center justify-center rounded transition-colors ${
                align === value ? "bg-sky-50 text-sky-700" : "text-slate-500 hover:bg-slate-100 hover:text-slate-900"
            }`}
        >
            <Icon className="w-3 h-3" />
        </button>
    );

    return createPortal(
        <motion.div
            data-floating=""
            initial={{ opacity: 0, y: 4 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.12 }}
            style={{ position: "fixed", top: Math.max(8, anchor.top - 40), left: anchor.left, zIndex: 60 }}
            ref={(el) => {
                // Nothing in a floating bar may sit off-screen: on a narrow
                // viewport an image near the right edge would push it out.
                if (!el) return;
                const overflow = el.getBoundingClientRect().right - window.innerWidth + 8;
                if (overflow > 0) el.style.left = `${Math.max(8, anchor.left - overflow)}px`;
            }}
            className="flex items-center gap-1 rounded-md border border-slate-200 bg-white p-1 shadow-[0_12px_32px_-8px_rgba(15,23,42,0.18)]"
        >
            {IMAGE_SIZE_PRESETS.map((p) => (
                <button
                    key={p.label}
                    type="button"
                    title={p.title}
                    aria-pressed={width === p.width}
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => set({ width: p.width })}
                    className={`h-6 px-1.5 rounded text-[11px] font-medium transition-colors ${
                        width === p.width ? "bg-sky-50 text-sky-700" : "text-slate-500 hover:bg-slate-100 hover:text-slate-900"
                    }`}
                >
                    {p.label}
                </button>
            ))}
            <span className="mx-0.5 h-4 w-px bg-slate-200" />
            {alignBtn("left", AlignLeftIcon, "Align left")}
            {alignBtn("center", AlignCenterIcon, "Center")}
            {alignBtn("right", AlignRightIcon, "Align right")}
            <span className="mx-0.5 h-4 w-px bg-slate-200" />
            <input
                value={alt}
                onChange={(e) => set({ alt: e.target.value })}
                placeholder="Alt text"
                title="Shown when the recipient's client blocks images, and read aloud by screen readers"
                className="h-6 w-32 rounded border border-slate-200 px-1.5 text-[11px] text-slate-800 outline-none focus:border-sky-400"
            />
            <button
                type="button"
                title="Remove image"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => editor.chain().focus().deleteSelection().run()}
                className="size-6 inline-flex items-center justify-center rounded text-slate-400 transition-colors hover:bg-rose-50 hover:text-rose-600"
            >
                <Trash2Icon className="w-3 h-3" />
            </button>
        </motion.div>,
        document.body,
    );
}
