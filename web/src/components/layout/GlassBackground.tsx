import { useAppStore } from '@/stores';

export function GlassBackground() {
    const glassBackground = useAppStore((state) => state.glassBackground);

    if (!glassBackground) return null;

    const bgUrl =
        glassBackground === "bg1"
            ? "/backdrops/bg1.png"
            : "/backdrops/bg2.png";

    return (
        <div
            className="fixed inset-0 z-[0] pointer-events-none"
            style={{
                backgroundImage: `url(${bgUrl})`,
                backgroundSize: "cover",
                backgroundPosition: "center",
                backgroundRepeat: "no-repeat",
                backgroundAttachment: "fixed",
            }}
        />
    );
}