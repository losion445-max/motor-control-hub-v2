import { useState } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Separator } from "@/components/ui/separator"
import { useWsConnection } from "@/hooks/useWsConnection"
import { useRobotStatus } from "@/hooks/useRobotStatus"
import { toast } from "@/components/ui/use-toast"

interface SettingsPageProps {
  rapidSpeed: number
  feedSpeed: number
  onRapidSpeedChange: (v: number) => void
  onFeedSpeedChange: (v: number) => void
}

export function SettingsPage({
  rapidSpeed,
  feedSpeed,
  onRapidSpeedChange,
  onFeedSpeedChange,
}: SettingsPageProps) {
  const { connectionState, url, reconnect } = useWsConnection()
  const status = useRobotStatus()
  const [wsUrl, setWsUrl] = useState(url)
  const [rapidInput, setRapidInput] = useState(String(rapidSpeed))
  const [feedInput, setFeedInput] = useState(String(feedSpeed))

  const handleReconnect = () => {
    reconnect(wsUrl)
    toast({ title: "Reconnecting…", description: wsUrl })
  }

  const handleSaveSpeeds = () => {
    const r = Number(rapidInput)
    const f = Number(feedInput)
    if (r > 0) {
      localStorage.setItem("rapid_speed", String(r))
      onRapidSpeedChange(r)
    }
    if (f > 0) {
      localStorage.setItem("feed_speed", String(f))
      onFeedSpeedChange(f)
    }
    toast({ title: "Speed presets saved" })
  }

  const connLabel =
    connectionState === "connected"
      ? "🟢 Connected"
      : connectionState === "connecting"
        ? "🟡 Connecting…"
        : "🔴 Disconnected"

  return (
    <div className="p-4 max-w-lg space-y-4">
      <Card>
        <CardHeader className="pb-2 pt-4 px-4">
          <CardTitle className="text-sm">WebSocket Connection</CardTitle>
        </CardHeader>
        <CardContent className="px-4 pb-4 space-y-3">
          <p className="text-sm">{connLabel}</p>
          <div className="space-y-1">
            <Label htmlFor="ws-url">URL</Label>
            <Input
              id="ws-url"
              value={wsUrl}
              onChange={(e) => setWsUrl(e.target.value)}
              placeholder="ws://localhost:8080/ws"
            />
          </div>
          <Button onClick={handleReconnect} variant="outline" className="w-full">
            Reconnect
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2 pt-4 px-4">
          <CardTitle className="text-sm">Speed Presets</CardTitle>
        </CardHeader>
        <CardContent className="px-4 pb-4 space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1">
              <Label htmlFor="rapid-spd">Rapid (G0) mm/s</Label>
              <Input
                id="rapid-spd"
                type="number"
                min={1}
                max={200}
                value={rapidInput}
                onChange={(e) => setRapidInput(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="feed-spd">Feed (G1) mm/s</Label>
              <Input
                id="feed-spd"
                type="number"
                min={1}
                max={200}
                value={feedInput}
                onChange={(e) => setFeedInput(e.target.value)}
              />
            </div>
          </div>
          <Button onClick={handleSaveSpeeds} className="w-full">
            Save presets
          </Button>
        </CardContent>
      </Card>

      {status && (
        <>
          <Separator />
          <Card>
            <CardHeader className="pb-2 pt-4 px-4">
              <CardTitle className="text-sm">Workspace Info</CardTitle>
            </CardHeader>
            <CardContent className="px-4 pb-4 space-y-1 text-sm font-mono">
              <Row label="Position X" value={`${status.x.toFixed(1)} mm`} />
              <Row label="Position Y" value={`${status.y.toFixed(1)} mm`} />
              <Row label="Homed" value={status.homed ? "Yes" : "No"} />
              <Row label="Busy" value={status.busy ? "Yes" : "No"} />
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between">
      <span className="text-muted-foreground">{label}</span>
      <span>{value}</span>
    </div>
  )
}
