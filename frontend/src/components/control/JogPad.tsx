import { useState } from "react"
import {
  ArrowUp,
  ArrowDown,
  ArrowLeft,
  ArrowRight,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { useCommand } from "@/hooks/useCommand"
import { toast } from "@/components/ui/use-toast"

const STEPS = [1, 10, 100] as const
type Step = (typeof STEPS)[number]

interface JogPadProps {
  currentX: number
  currentY: number
  feedSpeed: number
  disabled?: boolean
}

export function JogPad({ currentX, currentY, feedSpeed, disabled }: JogPadProps) {
  const [step, setStep] = useState<Step>(10)
  const { run, pending } = useCommand()

  const jog = (dx: number, dy: number) => {
    const nx = Math.min(1400, Math.max(0, currentX + dx))
    const ny = Math.min(2400, Math.max(0, currentY + dy))
    run("line", { x: nx, y: ny, speed: feedSpeed })
    toast({ title: `Jog → (${nx}, ${ny})` })
  }

  const isDisabled = disabled || pending

  return (
    <div className="space-y-2">
      <div className="flex gap-1">
        {STEPS.map((s) => (
          <button
            key={s}
            onClick={() => setStep(s)}
            className={`flex-1 rounded px-2 py-1 text-xs font-medium border transition-colors ${
              step === s
                ? "bg-primary text-primary-foreground border-primary"
                : "bg-background border-border hover:bg-accent"
            }`}
          >
            {s} mm
          </button>
        ))}
      </div>

      <div className="grid grid-cols-3 gap-1 w-32 mx-auto">
        <div />
        <Button
          variant="outline"
          size="icon"
          disabled={isDisabled}
          onClick={() => jog(0, -step)}
          className="h-9 w-9"
        >
          <ArrowUp className="h-4 w-4" />
        </Button>
        <div />

        <Button
          variant="outline"
          size="icon"
          disabled={isDisabled}
          onClick={() => jog(-step, 0)}
          className="h-9 w-9"
        >
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex items-center justify-center text-xs text-muted-foreground">
          {step}
        </div>
        <Button
          variant="outline"
          size="icon"
          disabled={isDisabled}
          onClick={() => jog(step, 0)}
          className="h-9 w-9"
        >
          <ArrowRight className="h-4 w-4" />
        </Button>

        <div />
        <Button
          variant="outline"
          size="icon"
          disabled={isDisabled}
          onClick={() => jog(0, step)}
          className="h-9 w-9"
        >
          <ArrowDown className="h-4 w-4" />
        </Button>
        <div />
      </div>
    </div>
  )
}
