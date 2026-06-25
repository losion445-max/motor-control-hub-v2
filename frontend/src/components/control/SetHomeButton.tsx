import { useEffect } from "react"
import { MapPin } from "lucide-react"
import { Button } from "@/components/ui/button"
import { useCommand } from "@/hooks/useCommand"
import { toast } from "@/components/ui/use-toast"

interface SetHomeButtonProps {
  disabled?: boolean
}

export function SetHomeButton({ disabled }: SetHomeButtonProps) {
  const { run, pending, error, messages } = useCommand()

  useEffect(() => {
    const last = messages.at(-1)
    if (last) toast({ title: last })
  }, [messages])

  useEffect(() => {
    if (error) toast({ title: "Set Home failed", description: error, variant: "destructive" })
  }, [error])

  const handleSetHome = () => {
    run("set_home")
  }

  return (
    <Button
      onClick={handleSetHome}
      disabled={disabled || pending}
      variant="outline"
      className="w-full gap-2"
    >
      <MapPin className="h-4 w-4" />
      {pending ? "Setting…" : "Set Home (no tension)"}
    </Button>
  )
}
