import { useEffect } from 'react';
import { useAppStore } from '@/stores';

export function GlassBackground() {
    const glassBackground = useAppStore((state) => state.glassBackground);

    useEffect(() => {
        const root = document.documentElement;
        
        // Toggle background classes on document root
        root.classList.remove('glass-bg-1', 'glass-bg-2');
        if (glassBackground === 'bg1') {
            root.classList.add('glass-bg-1');
        } else if (glassBackground === 'bg2') {
            root.classList.add('glass-bg-2');
        }
    }, [glassBackground]);

    if (!glassBackground) return null;

    return (
        <div className="fixed inset-0 z-0 pointer-events-none" />
    );
}