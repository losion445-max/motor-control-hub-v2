import { Badge } from "@/components/ui/badge"
import { useWsContext } from "@/context/WsContext"
import { useCommand } from "@/hooks/useCommand"
import { cn } from "@/lib/utils"

export function TopBar() {
  const { connectionState, status } = useWsContext()
  const { run } = useCommand()

  const connColor =
    connectionState === "connected"
      ? "bg-green-500"
      : connectionState === "connecting"
        ? "bg-yellow-500"
        : "bg-red-500"

  const connLabel =
    connectionState === "connected"
      ? "Connected"
      : connectionState === "connecting"
        ? "Connecting…"
        : "Disconnected"

  return (
    <header className="flex items-center gap-3 px-4 py-2 border-b bg-background sticky top-0 z-40">
      <div className="flex items-center gap-2">
        <span className={cn("h-2.5 w-2.5 rounded-full", connColor)} />
        <span className="text-sm text-muted-foreground">{connLabel}</span>
      </div>

      <div className="flex items-center gap-1 ml-4 text-sm font-mono">
        <span className="text-muted-foreground">X:</span>
        <span>{status ? status.x.toFixed(0) : "—"}</span>
        <span className="text-muted-foreground ml-2">Y:</span>
        <span>{status ? status.y.toFixed(0) : "—"}</span>
        <span className="text-muted-foreground ml-1">mm</span>
      </div>

      <div className="flex items-center gap-2 ml-4">
        {status?.homed ? (
          <Badge variant="secondary">Homed</Badge>
        ) : (
          <Badge variant="outline">Not Homed</Badge>
        )}
        {status?.busy && <Badge>Busy</Badge>}
      </div>

      <div className="ml-auto">
        <button
          onClick={() => run("stop")}
          className="px-4 py-1.5 rounded-md bg-destructive text-destructive-foreground text-sm font-bold hover:bg-destructive/90 transition-colors"
        >
          STOP
        </button>
      </div>
    </header>
  )
}
