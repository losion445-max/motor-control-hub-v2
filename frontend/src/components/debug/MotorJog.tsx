import { useState } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useWsContext } from "@/context/WsContext"
import { toast } from "@/components/ui/use-toast"

const MOTORS = [
  { id: 1, label: "M1", pos: "top-left" },
  { id: 2, label: "M2", pos: "top-right" },
  { id: 3, label: "M3", pos: "bot-right" },
  { id: 4, label: "M4", pos: "bot-left" },
]

function MotorCard({ motor }: { motor: (typeof MOTORS)[0] }) {
  const { client } = useWsContext()
  const [rpm, setRpm] = useState("25")
  const [jogging, setJogging] = useState<"wind" | "unwind" | null>(null)

  const jog = (dir: "wind" | "unwind") => {
    const speed = Math.abs(Number(rpm)) || 25
    const directedRpm = dir === "wind" ? speed : -speed
    const sent = client.sendNow({
      id: crypto.randomUUID(),
      cmd: "jog_start",
      motor: motor.id,
      rpm: directedRpm,
    })
    if (sent) {
      setJogging(dir)
    } else {
      toast({ title: "Not connected", variant: "destructive" })
    }
  }

  const stop = () => {
    client.sendNow({ id: crypto.randomUUID(), cmd: "jog_stop", motor: motor.id })
    setJogging(null)
  }

  return (
    <Card className="flex-1 min-w-[140px]">
      <CardHeader className="pb-1 pt-3 px-3">
        <CardTitle className="text-sm flex items-center gap-2">
          <span className="font-mono font-bold">{motor.label}</span>
          <span className="text-xs text-muted-foreground font-normal">{motor.pos}</span>
          {jogging && (
            <span className="ml-auto text-xs px-1.5 py-0.5 rounded bg-amber-100 text-amber-700 font-medium">
              {jogging === "wind" ? "↑ winding" : "↓ unwinding"}
            </span>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent className="px-3 pb-3 flex flex-col gap-2">
        <div className="space-y-1">
          <Label className="text-xs">RPM</Label>
          <Input
            type="number"
            min={1}
            max={200}
            value={rpm}
            onChange={(e) => setRpm(e.target.value)}
            className="h-7 text-sm"
          />
        </div>
        <Button
          size="sm"
          variant={jogging === "wind" ? "default" : "outline"}
          className="w-full h-7 text-xs"
          onClick={() => (jogging === "wind" ? stop() : jog("wind"))}
        >
          {jogging === "wind" ? "■ Stop" : "↑ Wind in"}
        </Button>
        <Button
          size="sm"
          variant={jogging === "unwind" ? "default" : "outline"}
          className="w-full h-7 text-xs"
          onClick={() => (jogging === "unwind" ? stop() : jog("unwind"))}
        >
          {jogging === "unwind" ? "■ Stop" : "↓ Unwind"}
        </Button>
        {jogging && (
          <Button size="sm" variant="destructive" className="w-full h-7 text-xs" onClick={stop}>
            STOP
          </Button>
        )}
      </CardContent>
    </Card>
  )
}

export function MotorJog() {
  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">
        Управление отдельными моторами. <strong>Не требует Home.</strong> Направление уже учитывает инверсию двигателя
        (Wind = кабель короче, Unwind = кабель длиннее).
      </p>
      <div className="flex gap-2 flex-wrap">
        {MOTORS.map((m) => (
          <MotorCard key={m.id} motor={m} />
        ))}
      </div>
    </div>
  )
}
