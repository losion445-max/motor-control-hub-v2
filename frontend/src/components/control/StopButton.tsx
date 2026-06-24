import { useWsContext } from "@/context/WsContext"
import { toast } from "@/components/ui/use-toast"
import { cn } from "@/lib/utils"

export function StopButton() {
  const { client, connectionState } = useWsContext()
  const connected = connectionState === "connected"

  const handleStop = () => {
    const id = crypto.randomUUID()
    const sent = client.sendNow({ id, cmd: "stop" })
    if (sent) {
      toast({ title: "Emergency stop sent", variant: "destructive" })
    } else {
      toast({
        title: "Not connected — stop NOT sent",
        description: "WebSocket is disconnected. Reconnecting…",
        variant: "destructive",
      })
    }
  }

  return (
    <button
      onClick={handleStop}
      className={cn(
        "w-full h-16 rounded-lg text-xl font-bold tracking-widest transition-all active:scale-95",
        connected
          ? "bg-destructive text-destructive-foreground hover:bg-destructive/90"
          : "bg-destructive/40 text-destructive-foreground/60 cursor-not-allowed",
      )}
    >
      {connected ? "STOP" : "STOP (offline)"}
    </button>
  )
}
