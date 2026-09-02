// BackgroundLayer — a fixed, pointer-events-none layer behind the entire
// application that displays the user-selected background image and
// responds to blur/opacity preferences stored in the appearance state.
//
// Sits at z-index: 0 behind the entire app shell. CSS variables on :root
// drive the visual values so the layer never causes layout thrashing.
// Toggles the `appearance-bg-active` class on <html> so other surfaces
// can adjust their own opacity when a background is present.

import { useEffect } from 'react'
import { useAppStore } from '@/stores'

export function BackgroundLayer() {
  const backgroundPreset = useAppStore((state) => state.backgroundPreset)
  const backgroundImage = useAppStore((state) => state.backgroundImage)
  const backgroundBlur = useAppStore((state) => state.backgroundBlur)
  const backgroundOpacity = useAppStore((state) => state.backgroundOpacity)

  const hasImage = backgroundImage || backgroundPreset !== 'default'

  useEffect(() => {
    const root = document.documentElement
    if (!hasImage) {
      root.style.setProperty('--app-bg-image', 'none')
      root.style.setProperty('--app-bg-blur', '0px')
      root.style.setProperty('--app-bg-opacity', '1')
      root.classList.remove('appearance-bg-active')
      return
    }

    const presetGradients: Record<string, string> = {
      'gradient-1': 'linear-gradient(135deg, #e0f2fe 0%, #f0f9ff 40%, #fef3c7 100%)',
      'gradient-2': 'linear-gradient(160deg, #f0f9ff 0%, #f5f3ff 50%, #fce7f3 100%)',
      'gradient-3': 'linear-gradient(145deg, #ecfdf5 0%, #f0fdf4 40%, #eff6ff 100%)',
    }

    const raw = backgroundImage || presetGradients[backgroundPreset] || 'none'
    const isGradient = !backgroundImage && backgroundPreset !== 'default'
    root.style.setProperty('--app-bg-image', isGradient ? raw : `url("${raw}")`)
    root.style.setProperty('--app-bg-blur', `${backgroundBlur}px`)
    root.style.setProperty('--app-bg-opacity', `${(backgroundOpacity / 100).toFixed(2)}`)
    root.classList.add('appearance-bg-active')
  }, [backgroundImage, backgroundPreset, backgroundBlur, backgroundOpacity, hasImage])

  if (!hasImage) return null

  return <div className="appearance-bg-layer" aria-hidden="true" />
}
