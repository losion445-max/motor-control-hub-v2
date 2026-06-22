import { useCommand } from "@/hooks/useCommand"
import { toast } from "@/components/ui/use-toast"

export function StopButton() {
  const { run } = useCommand()

  const handleStop = () => {
    run("stop")
    toast({ title: "Emergency stop sent", variant: "destructive" })
  }

  return (
    <button
      onClick={handleStop}
      className="w-full h-16 rounded-lg bg-destructive text-destructive-foreground text-xl font-bold tracking-widest hover:bg-destructive/90 active:scale-95 transition-all"
    >
      STOP
    </button>
  )
}
