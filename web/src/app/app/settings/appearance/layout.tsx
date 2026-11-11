import React from "react";
import { useAppStore } from "@/stores";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

export default function AppearanceSettings() {
  const theme = useAppStore((state) => state.theme);
  const setTheme = useAppStore((state) => state.setTheme);
  const glassmorphismEnabled = useAppStore((state) => state.glassmorphismEnabled);
  const setGlassmorphismEnabled = useAppStore((state) => state.setGlassmorphismEnabled);
  
  // Glassmorphism background selection
  const glassBackground = useAppStore((state) => state.glassBackground);
  const setGlassBackground = useAppStore((state) => state.setGlassBackground);
  
  // Glassmorphism advanced settings
  const glassBlurLevel = useAppStore((state) => state.glassBlurLevel);
  const setGlassBlurLevel = useAppStore((state) => state.setGlassBlurLevel);
  const glassTransparency = useAppStore((state) => state.glassTransparency);
  const setGlassTransparency = useAppStore((state) => state.setGlassTransparency);
  
  const handleThemeChange = (newTheme: "light" | "dark" | "system") => {
    setTheme(newTheme);
  };
  
  const handleBackgroundChange = (bg: "bg1" | "bg2") => {
    if (setGlassBackground) {
      setGlassBackground(bg);
    }
  };
  
  const handleBlurChange = (value: string) => {
    if (setGlassBlurLevel) {
      setGlassBlurLevel(parseInt(value));
    }
  };
  
  const handleTransparencyChange = (value: string) => {
    if (setGlassTransparency) {
      setGlassTransparency(parseInt(value));
    }
  };
  
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">Appearance</h2>
        <p className="text-sm text-slate-500 mt-1">
          Customize how Warmbly looks and feels.
        </p>
      </div>
      
      <Card>
        <CardHeader>
          <CardTitle>Theme</CardTitle>
          <CardDescription>
            Choose your preferred color scheme
          </CardDescription>
        </CardHeader>
        <CardContent>
            <div role="radiogroup" aria-label="Theme" className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <button
                type="button"
                role="radio"
                aria-checked={theme === "light"}
                tabIndex={theme === "light" ? 0 : -1}
                onClick={() => handleThemeChange("light")}
                className={cn(
                  "flex items-center space-x-2 p-3 rounded-lg border outline-none transition-colors",
                  theme === "light"
                    ? "border-sky-500 bg-sky-50 text-sky-700"
                    : "border-slate-200 hover:border-slate-300 text-slate-700"
                )}
              >
                <span className="h-4 w-4 rounded-full border border-slate-300 flex items-center justify-center">
                  {theme === "light" && <span className="h-2 w-2 rounded-full bg-sky-500" />}
                </span>
                <span className="font-medium">Light</span>
              </button>
              <button
                type="button"
                role="radio"
                aria-checked={theme === "dark"}
                tabIndex={theme === "dark" ? 0 : -1}
                onClick={() => handleThemeChange("dark")}
                className={cn(
                  "flex items-center space-x-2 p-3 rounded-lg border outline-none transition-colors",
                  theme === "dark"
                    ? "border-sky-500 bg-sky-50 text-sky-700"
                    : "border-slate-200 hover:border-slate-300 text-slate-700"
                )}
              >
                <span className="h-4 w-4 rounded-full border border-slate-300 flex items-center justify-center">
                  {theme === "dark" && <span className="h-2 w-2 rounded-full bg-sky-500" />}
                </span>
                <span className="font-medium">Dark</span>
              </button>
              <button
                type="button"
                role="radio"
                aria-checked={theme === "system"}
                tabIndex={theme === "system" ? 0 : -1}
                onClick={() => handleThemeChange("system")}
                className={cn(
                  "flex items-center space-x-2 p-3 rounded-lg border outline-none transition-colors",
                  theme === "system"
                    ? "border-sky-500 bg-sky-50 text-sky-700"
                    : "border-slate-200 hover:border-slate-300 text-slate-700"
                )}
              >
                <span className="h-4 w-4 rounded-full border border-slate-300 flex items-center justify-center">
                  {theme === "system" && <span className="h-2 w-2 rounded-full bg-sky-500" />}
                </span>
                <span className="font-medium">System</span>
              </button>
              <div className="flex items-center justify-between py-2 px-3 rounded-lg border border-slate-200 hover:border-slate-300 text-slate-700">
                <div className="flex items-center space-x-2">
                  <span className="h-4 w-4 rounded-full border border-slate-300 flex items-center justify-center">
                    {glassmorphismEnabled && <span className="h-2 w-2 rounded-full bg-sky-500" />}
                  </span>
                  <span className="font-medium">Glassmorphism</span>
                </div>
                <button 
                  type="button"
                  onClick={() => setGlassmorphismEnabled(!glassmorphismEnabled)}
                  className="relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-sky-500 focus:ring-offset-2"
                  aria-label="Toggle Glassmorphism"
                >
                  <span
                    className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${glassmorphismEnabled ? 'translate-x-6' : 'translate-x-1'}`}
                  />
                </button>
              </div>
            </div>
        </CardContent>
      </Card>
      
      {glassmorphismEnabled && (
        <Card>
          <CardHeader>
            <CardTitle>Glassmorphism Settings</CardTitle>
            <CardDescription>
              Configure your glassmorphism experience
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <Label htmlFor="background">Background Image</Label>
              <Select value={glassBackground || 'bg1'} onValueChange={handleBackgroundChange}>
                <SelectTrigger id="background" className="w-full">
                  <SelectValue placeholder="Select background" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="bg1">Background 1</SelectItem>
                  <SelectItem value="bg2">Background 2</SelectItem>
                </SelectContent>
              </Select>
            </div>
            
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div className="relative rounded-lg overflow-hidden border border-slate-200 bg-slate-50">
                <div className="absolute inset-0 bg-gradient-to-t from-black/70 to-transparent"></div>
                <div className="absolute bottom-0 left-0 right-0 p-4 text-white z-10">
                  <h3 className="font-medium text-sm">Background 1</h3>
                  <p className="text-xs opacity-80">Subtle pattern</p>
                </div>
                <img 
                  src="/backdrops/bg1.png" 
                  alt="Background 1" 
                  className="w-full h-32 object-cover"
                />
              </div>
              
              <div className="relative rounded-lg overflow-hidden border border-slate-200 bg-slate-50">
                <div className="absolute inset-0 bg-gradient-to-t from-black/70 to-transparent"></div>
                <div className="absolute bottom-0 left-0 right-0 p-4 text-white z-10">
                  <h3 className="font-medium text-sm">Background 2</h3>
                  <p className="text-xs opacity-80">Abstract gradient</p>
                </div>
                <img 
                  src="/backdrops/bg2.png" 
                  alt="Background 2" 
                  className="w-full h-32 object-cover"
                />
              </div>
            </div>
          </CardContent>
        </Card>
      )}
      
      <Card>
        <CardHeader>
          <CardTitle>Advanced Options</CardTitle>
          <CardDescription>
            Fine-tune your glassmorphism experience
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            <div>
              <Label htmlFor="blur-level">Backdrop Blur Level</Label>
              <Input id="blur-level" type="range" min="8" max="24" value={glassBlurLevel} onChange={(e) => handleBlurChange(e.target.value)} />
              <div className="flex justify-between text-xs text-slate-500 mt-1">
                <span>Subtle</span>
                <span>Strong</span>
              </div>
            </div>
            
            <div>
              <Label htmlFor="transparency">Transparency Level</Label>
              <Input id="transparency" type="range" min="40" max="90" value={glassTransparency} onChange={(e) => handleTransparencyChange(e.target.value)} />
              <div className="flex justify-between text-xs text-slate-500 mt-1">
                <span>Opaque</span>
                <span>Transparent</span>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}