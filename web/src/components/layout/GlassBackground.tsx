import { useEffect } from 'react';
import { useAppStore } from '@/stores';

export function GlassBackground() {
    const glassBackground = useAppStore((state) => state.glassBackground);
    const glassmorphismEnabled = useAppStore((state) => state.glassmorphismEnabled);

    useEffect(() => {
        const html = document.querySelector('html');
        if (!html) return;
        
        // Toggle background classes on html element
        html.classList.remove('glass-bg-1', 'glass-bg-2');
        if (glassmorphismEnabled && glassBackground === 'bg1') {
            html.classList.add('glass-bg-1');
        } else if (glassmorphismEnabled && glassBackground === 'bg2') {
            html.classList.add('glass-bg-2');
        }
    }, [glassBackground, glassmorphismEnabled]);

    if (!glassmorphismEnabled || !glassBackground) return null;

    return (
        <div className="fixed inset-0 z-0 pointer-events-none" />
    );
}