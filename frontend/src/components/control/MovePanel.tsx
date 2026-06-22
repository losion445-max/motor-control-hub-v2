import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useCommand } from "@/hooks/useCommand"
import { toast } from "@/components/ui/use-toast"

interface MovePanelProps {
  disabled?: boolean
  x: string
  y: string
  speed: string
  onXChange: (v: string) => void
  onYChange: (v: string) => void
  onSpeedChange: (v: string) => void
  rapidSpeed: number
  feedSpeed: number
}

export function MovePanel({
  disabled,
  x,
  y,
  speed,
  onXChange,
  onYChange,
  onSpeedChange,
  rapidSpeed,
  feedSpeed,
}: MovePanelProps) {
  const { run, pending } = useCommand()

  const clamp = (val: string, min: number, max: number) =>
    Math.min(max, Math.max(min, Number(val)))

  const handleRapid = () => {
    const px = clamp(x, 0, 1400)
    const py = clamp(y, 0, 2400)
    const spd = speed ? Number(speed) : rapidSpeed
    run("move", { x: px, y: py, speed: spd })
    toast({ title: `Rapid → (${px}, ${py}) at ${spd} mm/s` })
  }

  const handleLine = () => {
    const px = clamp(x, 0, 1400)
    const py = clamp(y, 0, 2400)
    const spd = speed ? Number(speed) : feedSpeed
    run("line", { x: px, y: py, speed: spd })
    toast({ title: `Line → (${px}, ${py}) at ${spd} mm/s` })
  }

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-3 gap-2">
        <div className="space-y-1">
          <Label htmlFor="mv-x">X mm</Label>
          <Input
            id="mv-x"
            type="number"
            min={0}
            max={1400}
            value={x}
            onChange={(e) => onXChange(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="mv-y">Y mm</Label>
          <Input
            id="mv-y"
            type="number"
            min={0}
            max={2400}
            value={y}
            onChange={(e) => onYChange(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="mv-spd">Speed mm/s</Label>
          <Input
            id="mv-spd"
            type="number"
            min={1}
            max={200}
            placeholder={String(feedSpeed)}
            value={speed}
            onChange={(e) => onSpeedChange(e.target.value)}
          />
        </div>
      </div>
      <div className="grid grid-cols-2 gap-2">
        <Button onClick={handleRapid} disabled={disabled || pending}>
          Rapid (G0)
        </Button>
        <Button
          onClick={handleLine}
          disabled={disabled || pending}
          variant="outline"
        >
          Line (G1)
        </Button>
      </div>
    </div>
  )
}
