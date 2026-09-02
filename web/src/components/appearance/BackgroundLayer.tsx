// BackgroundLayer — a fixed, pointer-events-none layer behind the entire
// application that displays the user-selected background image and
// responds to blur/opacity preferences stored in the appearance state.
//
// Lives as the first child of AppShell so it stays behind the chrome
// and content panels. CSS variables on :root drive the visual values
// so the layer never causes layout thrashing.

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
      return
    }

    const presetGradients: Record<string, string> = {
      'gradient-1': 'linear-gradient(135deg, #e0f2fe 0%, #f0f9ff 40%, #fef3c7 100%)',
      'gradient-2': 'linear-gradient(160deg, #f0f9ff 0%, #f5f3ff 50%, #fce7f3 100%)',
      'gradient-3': 'linear-gradient(145deg, #ecfdf5 0%, #f0fdf4 40%, #eff6ff 100%)',
    }

    const url = backgroundImage || presetGradients[backgroundPreset] || 'none'
    root.style.setProperty('--app-bg-image', `url("${url}")`)
    root.style.setProperty('--app-bg-blur', `${backgroundBlur}px`)
    root.style.setProperty('--app-bg-opacity', `${(backgroundOpacity / 100).toFixed(2)}`)
  }, [backgroundImage, backgroundPreset, backgroundBlur, backgroundOpacity, hasImage])

  if (!hasImage) return null

  return <div className="appearance-bg-layer" aria-hidden="true" />
}
