import { useState } from "react"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import { Toaster } from "@/components/ui/toaster"
import { TopBar } from "@/components/layout/TopBar"
import { WsProvider } from "@/context/WsContext"
import { ControlPage } from "@/pages/ControlPage"
import { GcodePage } from "@/pages/GcodePage"
import { SettingsPage } from "@/pages/SettingsPage"
import { DebugPage } from "@/pages/DebugPage"

function loadNum(key: string, fallback: number): number {
  const v = localStorage.getItem(key)
  return v ? Number(v) : fallback
}

export default function App() {
  const [rapidSpeed, setRapidSpeed] = useState(() => loadNum("rapid_speed", 50))
  const [feedSpeed, setFeedSpeed] = useState(() => loadNum("feed_speed", 20))

  return (
    <WsProvider>
      <div className="flex flex-col h-screen">
        <TopBar />
        <Tabs defaultValue="control" className="flex-1 flex flex-col min-h-0">
          <div className="px-4 pt-2 border-b">
            <TabsList>
              <TabsTrigger value="control">Control</TabsTrigger>
              <TabsTrigger value="gcode">G-code</TabsTrigger>
              <TabsTrigger value="settings">Settings</TabsTrigger>
              <TabsTrigger value="debug">Debug</TabsTrigger>
            </TabsList>
          </div>
          <TabsContent value="control" className="flex-1 min-h-0 mt-0">
            <ControlPage rapidSpeed={rapidSpeed} feedSpeed={feedSpeed} />
          </TabsContent>
          <TabsContent value="gcode" className="flex-1 min-h-0 mt-0">
            <GcodePage />
          </TabsContent>
          <TabsContent value="settings" className="flex-1 min-h-0 mt-0 overflow-y-auto">
            <SettingsPage
              rapidSpeed={rapidSpeed}
              feedSpeed={feedSpeed}
              onRapidSpeedChange={setRapidSpeed}
              onFeedSpeedChange={setFeedSpeed}
            />
          </TabsContent>
          <TabsContent value="debug" className="flex-1 min-h-0 mt-0">
            <DebugPage />
          </TabsContent>
        </Tabs>
      </div>
      <Toaster />
    </WsProvider>
  )
}
