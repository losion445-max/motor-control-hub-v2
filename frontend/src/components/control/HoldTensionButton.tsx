import { Zap } from "lucide-react"
import { Button } from "@/components/ui/button"
import { useCommand } from "@/hooks/useCommand"
import { toast } from "@/components/ui/use-toast"

interface HoldTensionButtonProps {
  disabled?: boolean
}

export function HoldTensionButton({ disabled }: HoldTensionButtonProps) {
  const { run, pending } = useCommand()

  const handleHold = () => {
    run("hold_tension")
    toast({ title: "Hold tension active", description: "Passive cable tension enabled" })
  }

  return (
    <Button
      onClick={handleHold}
      disabled={disabled || pending}
      variant="secondary"
      className="w-full gap-2"
    >
      <Zap className="h-4 w-4" />
      {pending ? "Activating…" : "Hold Tension"}
    </Button>
  )
}
