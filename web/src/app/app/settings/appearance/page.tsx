// Appearance settings — theme, glassmorphism, and background controls.
//
// All values are persisted client-side via the appearance slice in
// the Zustand store; nothing is sent to the backend.

import React from "react";
import { CheckIcon, ImageIcon, TrashIcon, SunIcon, MoonIcon, MonitorIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import {
    Row,
    Section,
    SectionShell,
    ToggleRow,
} from "../_components/SectionShell";
import { DitherSlider } from "@/components/ui/dither";
import { useAppStore } from "@/stores";
import { usePermission } from "@/hooks/usePermission";
import { NoAccess } from "@/components/layout/NoAccess";

const THEME_OPTIONS = [
    { value: "light", label: "Light", icon: <SunIcon className="w-3.5 h-3.5 text-amber-500" /> },
    { value: "dark", label: "Dark", icon: <MoonIcon className="w-3.5 h-3.5 text-indigo-400" /> },
    { value: "system", label: "System", icon: <MonitorIcon className="w-3.5 h-3.5 text-slate-500" /> },
] as const;

const PRESET_OPTIONS = [
    { value: "default", label: "Default" },
    { value: "gradient-1", label: "Sky fade" },
    { value: "gradient-2", label: "Lavender" },
    { value: "gradient-3", label: "Mint" },
] as const;

    const PRESET_BACKGROUNDS: Record<string, string> = {
        "gradient-1": "linear-gradient(170deg, #f8fafc 0%, #e0f2fe 18%, #fef3c7 48%, #fde68a 78%, #fefce8 100%)",
        "gradient-2": "linear-gradient(165deg, #faf5ff 0%, #f3e8ff 22%, #fce7f3 52%, #fbcfe8 82%, #fdf2f8 100%)",
        "gradient-3": "linear-gradient(150deg, #f0fdf4 0%, #d1fae5 22%, #a7f3d0 52%, #e0f2fe 82%, #f0f9ff 100%)",
    };

export default function AppearanceSettingsPage() {
    const canManage = usePermission("MANAGE_SETTINGS");
    if (!canManage) return <NoAccess feature="Appearance" permissionLabel="Manage settings" />;
    return <AppearanceSettings />;
}

function AppearanceSettings() {
    const resolvedTheme = useAppStore((state) => state.resolvedTheme);
    const setTheme = useAppStore((state) => state.setTheme);
    const glassmorphismEnabled = useAppStore((state) => state.glassmorphismEnabled);
    const glassOpacity = useAppStore((state) => state.glassOpacity);
    const glassBlur = useAppStore((state) => state.glassBlur);
    const backgroundPreset = useAppStore((state) => state.backgroundPreset);
    const backgroundImage = useAppStore((state) => state.backgroundImage);
    const backgroundBlur = useAppStore((state) => state.backgroundBlur);
    const backgroundOpacity = useAppStore((state) => state.backgroundOpacity);
    const setGlassmorphismEnabled = useAppStore((state) => state.setGlassmorphismEnabled);
    const setGlassOpacity = useAppStore((state) => state.setGlassOpacity);
    const setGlassBlur = useAppStore((state) => state.setGlassBlur);
    const setBackgroundPreset = useAppStore((state) => state.setBackgroundPreset);
    const setBackgroundImage = useAppStore((state) => state.setBackgroundImage);
    const setBackgroundBlur = useAppStore((state) => state.setBackgroundBlur);
    const setBackgroundOpacity = useAppStore((state) => state.setBackgroundOpacity);

    const isDark = resolvedTheme === "dark";
    const hasCustomImage = Boolean(backgroundImage);

    return (
        <SectionShell
            title="Appearance"
            description="Customize how Warmbly looks for you. Changes apply instantly."
        >
            <div className="px-4 py-4 md:px-8 md:py-5">
                <AppearancePreview
                    isDark={isDark}
                    glassmorphismEnabled={glassmorphismEnabled}
                    glassOpacity={glassOpacity}
                    glassBlur={glassBlur}
                    backgroundPreset={backgroundPreset}
                    backgroundImage={backgroundImage}
                    backgroundBlur={backgroundBlur}
                    backgroundOpacity={backgroundOpacity}
                />
            </div>

            <Section
                eyebrow="Theme"
                description="Choose between light, dark, or follow your system preference."
            >
                <Row label="Appearance">
                    <div className="inline-flex rounded-lg border border-slate-200 bg-slate-50/60 p-0.5">
                        {THEME_OPTIONS.map((opt) => {
                            const active = resolvedTheme === opt.value;
                            return (
                                <button
                                    key={opt.value}
                                    onClick={() => setTheme(opt.value)}
                                    className={cn(
                                        "inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-[12px] font-medium transition-all duration-200",
                                        active
                                            ? "bg-white text-slate-900 shadow-sm border border-slate-200/80"
                                            : "text-slate-500 hover:text-slate-700 border border-transparent"
                                    )}
                                >
                                    {opt.icon}
                                    <span>{opt.label}</span>
                                </button>
                            );
                        })}
                    </div>
                </Row>
            </Section>

            <Section
                eyebrow="Glassmorphism"
                description="Add a subtle frosted-glass effect to panels, cards, and dialogs. Off by default."
            >
                <ToggleRow
                    label="Enable glassmorphism"
                    description="Applies a translucent, blurred surface to the sidebar, navigation, cards, and dialogs."
                    checked={glassmorphismEnabled}
                    onChange={setGlassmorphismEnabled}
                />
                {glassmorphismEnabled && (
                    <div className="mt-4 space-y-5">
                        <SliderRow
                            label="Glass opacity"
                            description="Surface transparency. Higher = more transparent."
                            value={glassOpacity}
                            onChange={setGlassOpacity}
                            min={60}
                            max={100}
                            unit="%"
                        />
                        <SliderRow
                            label="Glass blur"
                            description="Blur strength in pixels."
                            value={glassBlur}
                            onChange={setGlassBlur}
                            min={0}
                            max={40}
                            unit="px"
                        />
                    </div>
                )}
            </Section>

            <Section
                eyebrow="Background"
                description="Set a background image behind the application. Stored locally in your browser."
            >
                <div className="space-y-5">
                    <div>
                        <div className="text-[12.5px] font-medium text-slate-900 mb-2">Preset</div>
                        <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
                            {PRESET_OPTIONS.map((preset) => {
                                const active = backgroundPreset === preset.value && !hasCustomImage;
                                return (
                                    <button
                                        key={preset.value}
                                        onClick={() => {
                                            setBackgroundPreset(preset.value as typeof backgroundPreset);
                                            if (preset.value !== "default") {
                                                setBackgroundImage("");
                                            }
                                        }}
                                        className={cn(
                                            "relative rounded-lg border-2 p-1.5 text-left transition-all duration-200",
                                            active
                                                ? "border-sky-500 shadow-sm"
                                                : "border-slate-200 hover:border-slate-300"
                                        )}
                                    >
                                        <div
                                            className="h-10 rounded-md mb-1.5"
                                            style={{
                                                background:
                                                    preset.value === "default"
                                                        ? "var(--secondary)"
                                                        : PRESET_BACKGROUNDS[preset.value],
                                            }}
                                        />
                                        <div className="text-[11px] font-medium text-slate-700 px-0.5">
                                            {preset.label}
                                        </div>
                                        {active && (
                                            <div className="absolute top-1.5 right-1.5 size-4 rounded-full bg-sky-500 text-white flex items-center justify-center">
                                                <CheckIcon className="w-2.5 h-2.5" />
                                            </div>
                                        )}
                                    </button>
                                );
                            })}
                        </div>
                    </div>

                    <div>
                        <div className="text-[12.5px] font-medium text-slate-900 mb-2">Custom image</div>
                        <BackgroundImageUploader
                            current={backgroundImage}
                            onSelect={(url) => {
                                setBackgroundImage(url);
                                if (url) setBackgroundPreset("default");
                            }}
                            onClear={() => {
                                setBackgroundImage("");
                                setBackgroundPreset("default");
                            }}
                        />
                    </div>

                    <SliderRow
                        label="Background blur"
                        description="Blur applied to the background image."
                        value={backgroundBlur}
                        onChange={setBackgroundBlur}
                        min={0}
                        max={40}
                        unit="px"
                    />
                    <SliderRow
                        label="Background opacity"
                        description="Background visibility."
                        value={backgroundOpacity}
                        onChange={setBackgroundOpacity}
                        min={0}
                        max={100}
                        unit="%"
                    />
                </div>
            </Section>
        </SectionShell>
    );
}

function SliderRow({
    label,
    description,
    value,
    onChange,
    min,
    max,
    unit,
}: {
    label: string;
    description?: string;
    value: number;
    onChange: (v: number) => void;
    min: number;
    max: number;
    unit: string;
}) {
    return (
        <div className="space-y-2.5">
            <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                    <div className="text-[12.5px] font-medium text-slate-900">{label}</div>
                    {description && (
                        <div className="text-[11px] text-slate-500 mt-0.5 leading-relaxed">{description}</div>
                    )}
                </div>
                <span className="text-[12px] text-slate-600 font-mono tabular-nums min-w-[3.5ch] text-right shrink-0">
                    {value}
                    {unit}
                </span>
            </div>
            <DitherSlider
                value={value}
                min={min}
                max={max}
                step={1}
                onChange={onChange}
                tone="sky"
            />
        </div>
    );
}

function AppearancePreview({
    isDark,
    glassmorphismEnabled,
    glassOpacity,
    glassBlur,
    backgroundPreset,
    backgroundImage,
    backgroundBlur,
    backgroundOpacity,
}: {
    isDark: boolean;
    glassmorphismEnabled: boolean;
    glassOpacity: number;
    glassBlur: number;
    backgroundPreset: string;
    backgroundImage: string;
    backgroundBlur: number;
    backgroundOpacity: number;
}) {
    const bgStyle: React.CSSProperties = {};
    if (backgroundImage) {
        bgStyle.backgroundImage = `url(${backgroundImage})`;
        bgStyle.backgroundSize = "cover";
        bgStyle.backgroundPosition = "center";
        bgStyle.backgroundRepeat = "no-repeat";
    } else if (backgroundPreset !== "default" && PRESET_BACKGROUNDS[backgroundPreset]) {
        bgStyle.background = PRESET_BACKGROUNDS[backgroundPreset];
    }

    const surface = glassmorphismEnabled
        ? isDark
            ? `rgba(30, 30, 40, ${glassOpacity / 100})`
            : `rgba(255, 255, 255, ${glassOpacity / 100})`
        : isDark
          ? "rgb(22, 22, 30)"
          : "rgb(255, 255, 255)";

    const border = glassmorphismEnabled
        ? isDark
            ? "rgba(255, 255, 255, 0.08)"
            : "rgba(0, 0, 0, 0.06)"
        : isDark
          ? "rgb(51, 51, 65)"
          : "rgb(226, 232, 240)";

    const shadow = glassmorphismEnabled
        ? "0 8px 32px -8px rgba(0, 0, 0, 0.12)"
        : "none";

    return (
        <div className="relative h-40 w-full overflow-hidden rounded-xl border border-slate-200 bg-slate-100">
            <div
                className="absolute inset-0 transition-all duration-500 ease-out"
                style={{
                    ...bgStyle,
                    opacity: backgroundImage || backgroundPreset !== "default" ? backgroundOpacity / 100 : 1,
                    filter: backgroundBlur ? `blur(${backgroundBlur}px)` : undefined,
                }}
            />

            <div
                className="relative z-10 flex h-full transition-all duration-300"
                style={{
                    background: surface,
                    backdropFilter: glassmorphismEnabled ? `blur(${glassBlur}px)` : undefined,
                    WebkitBackdropFilter: glassmorphismEnabled ? `blur(${glassBlur}px)` : undefined,
                    borderLeft: `1px solid ${border}`,
                    borderBottom: `1px solid ${border}`,
                    boxShadow: shadow,
                }}
            >
                <div
                    className="w-12 border-r flex flex-col items-center py-3 gap-2 transition-colors duration-300"
                    style={{ borderColor: border }}
                >
                    <div className={cn("size-4 rounded-md", isDark ? "bg-slate-700" : "bg-slate-200")} />
                    <div className={cn("size-4 rounded-md", isDark ? "bg-slate-800" : "bg-slate-50")} />
                    <div className={cn("size-4 rounded-md", isDark ? "bg-slate-800" : "bg-slate-50")} />
                </div>

                <div className="flex-1 p-4 flex flex-col gap-2.5">
                    <div
                        className={cn("h-2 rounded-full transition-colors duration-300", isDark ? "bg-slate-600" : "bg-slate-200")}
                        style={{ width: "35%" }}
                    />
                    <div
                        className={cn("h-2 rounded-full transition-colors duration-300", isDark ? "bg-slate-700" : "bg-slate-100")}
                        style={{ width: "65%" }}
                    />

                    <div
                        className={cn(
                            "mt-auto rounded-lg p-3 transition-all duration-300",
                            glassmorphismEnabled
                                ? isDark
                                    ? "bg-white/10 border"
                                    : "bg-white/80 border shadow-sm"
                                : isDark
                                  ? "bg-slate-800 border"
                                  : "bg-slate-50 border"
                        )}
                        style={{ borderColor: glassmorphismEnabled ? (isDark ? "rgba(255,255,255,0.1)" : "rgba(0,0,0,0.04)") : undefined }}
                    >
                        <div
                            className={cn("h-1.5 rounded-full mb-2 transition-colors duration-300", isDark ? "bg-slate-600" : "bg-slate-200")}
                            style={{ width: "50%" }}
                        />
                        <div
                            className={cn("h-1.5 rounded-full transition-colors duration-300", isDark ? "bg-slate-700" : "bg-slate-100")}
                            style={{ width: "75%" }}
                        />
                    </div>
                </div>
            </div>
        </div>
    );
}

function BackgroundImageUploader({
    current,
    onSelect,
    onClear,
}: {
    current: string;
    onSelect: (url: string) => void;
    onClear: () => void;
}) {
    const inputRef = React.useRef<HTMLInputElement>(null);
    const [preview, setPreview] = React.useState<string | null>(current || null);
    const [dragging, setDragging] = React.useState(false);
    const [error, setError] = React.useState<string | null>(null);

    React.useEffect(() => {
        setPreview(current || null);
        setError(null);
    }, [current]);

    React.useEffect(() => {
        return () => {
            if (preview && preview !== current) URL.revokeObjectURL(preview);
        };
    }, [preview, current]);

    function handleFile(file: File) {
        setError(null);
        if (!file.type.startsWith("image/")) {
            setError("Please select an image file.");
            return;
        }
        if (file.size > 10 * 1024 * 1024) {
            setError("Image must be smaller than 10 MB.");
            return;
        }
        const url = URL.createObjectURL(file);
        setPreview(url);
        onSelect(url);
    }

    function onPicked(e: React.ChangeEvent<HTMLInputElement>) {
        const f = e.target.files?.[0];
        e.target.value = "";
        if (f) handleFile(f);
    }

    function onDrop(e: React.DragEvent) {
        e.preventDefault();
        setDragging(false);
        const f = e.dataTransfer.files?.[0];
        if (f) handleFile(f);
    }

    const displayUrl = preview || current;

    return (
        <div className="space-y-2">
            {displayUrl ? (
                <div className="flex items-center gap-3">
                    <div className="size-16 rounded-lg border border-slate-200 overflow-hidden shrink-0 bg-slate-50">
                        <img
                            src={displayUrl}
                            alt=""
                            className="w-full h-full object-cover"
                        />
                    </div>
                    <div className="flex items-center gap-1.5">
                        <button
                            type="button"
                            onClick={() => inputRef.current?.click()}
                            className="h-7 px-2.5 rounded-md border border-slate-200 hover:border-slate-300 text-[12px] text-slate-700 hover:text-slate-900 transition-colors inline-flex items-center gap-1.5"
                        >
                            <ImageIcon className="w-3 h-3" />
                            Replace
                        </button>
                        <button
                            type="button"
                            onClick={() => {
                                if (preview && preview !== current) URL.revokeObjectURL(preview);
                                setPreview(null);
                                setError(null);
                                onClear();
                            }}
                            className="h-7 px-2.5 rounded-md text-[12px] text-slate-500 hover:text-red-700 hover:bg-red-50 transition-colors inline-flex items-center gap-1.5"
                        >
                            <TrashIcon className="w-3 h-3" />
                            Remove
                        </button>
                    </div>
                </div>
            ) : (
                <div
                    onDragOver={(e) => {
                        e.preventDefault();
                        setDragging(true);
                    }}
                    onDragLeave={() => setDragging(false)}
                    onDrop={onDrop}
                    onClick={() => inputRef.current?.click()}
                    className={cn(
                        "flex flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed p-6 text-center cursor-pointer transition-colors",
                        dragging
                            ? "border-sky-400 bg-sky-50/50"
                            : "border-slate-200 hover:border-slate-300 hover:bg-slate-50/50"
                    )}
                >
                    <ImageIcon className="w-5 h-5 text-slate-400" />
                    <div>
                        <div className="text-[12px] text-slate-700 font-medium">Drop an image here</div>
                        <div className="text-[11px] text-slate-400 mt-0.5">or click to browse</div>
                    </div>
                    <div className="text-[10px] text-slate-400">PNG, JPG, WebP up to 10 MB</div>
                </div>
            )}
            {error && <div className="text-[11px] text-red-600">{error}</div>}
            <input
                ref={inputRef}
                type="file"
                accept="image/png,image/jpeg,image/jpg,image/webp"
                onChange={onPicked}
                className="hidden"
                aria-hidden="true"
            />
        </div>
    );
}
