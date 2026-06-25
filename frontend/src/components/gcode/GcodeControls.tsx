import { Play, Square } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Progress } from "@/components/ui/progress"
import { useCommand } from "@/hooks/useCommand"
import { useWsContext } from "@/context/WsContext"
import { toast } from "@/components/ui/use-toast"

interface GcodeControlsProps {
  content: string
  disabled?: boolean
  speedOverride: string
  onSpeedOverrideChange: (v: string) => void
}

export function GcodeControls({
  content,
  disabled,
  speedOverride,
  onSpeedOverrideChange,
}: GcodeControlsProps) {
  const { run, reset, pending, messages, error } = useCommand()
  const { client } = useWsContext()

  const totalLines = content.split("\n").filter((l) => l.trim()).length
  const doneLines = messages.length
  const progress = totalLines > 0 ? Math.round((doneLines / totalLines) * 100) : 0

  const handleRun = () => {
    reset()
    let program = content
    const pct = Number(speedOverride)
    if (pct > 0 && pct !== 100) {
      program = content
        .split("\n")
        .map((line) => {
          const m = line.match(/F([\d.]+)/i)
          if (m) {
            const newF = (Number(m[1]) * pct) / 100
            return line.replace(/F[\d.]+/i, `F${newF.toFixed(1)}`)
          }
          return line
        })
        .join("\n")
    }
    run("gcode", { program })
    toast({ title: "G-code started", description: `${totalLines} commands` })
  }

  const handleStop = () => {
    const sent = client.sendNow({ id: crypto.randomUUID(), cmd: "stop" })
    toast({
      title: sent ? "Stop sent" : "Not connected — stop NOT sent",
      variant: "destructive",
    })
  }

  return (
    <div className="space-y-3">
      <div className="flex items-end gap-3">
        <div className="space-y-1">
          <Label htmlFor="spd-override">Speed override %</Label>
          <Input
            id="spd-override"
            type="number"
            min={1}
            max={200}
            placeholder="100"
            className="w-28"
            value={speedOverride}
            onChange={(e) => onSpeedOverrideChange(e.target.value)}
          />
        </div>
        <Button
          onClick={handleRun}
          disabled={!content || disabled || pending}
          className="gap-2"
        >
          <Play className="h-4 w-4" />
          Run
        </Button>
        <Button
          onClick={handleStop}
          variant="destructive"
          disabled={!pending}
          className="gap-2"
        >
          <Square className="h-4 w-4" />
          Stop
        </Button>
      </div>

      {pending && (
        <div className="space-y-1">
          <Progress value={progress} className="h-2" />
          <p className="text-xs text-muted-foreground">
            {doneLines} / {totalLines} commands
          </p>
        </div>
      )}

      {error && (
        <p className="text-sm text-destructive">{error}</p>
      )}

      {messages.length > 0 && !pending && (
        <p className="text-sm text-muted-foreground">
          {messages[messages.length - 1]}
        </p>
      )}
    </div>
  )
}
