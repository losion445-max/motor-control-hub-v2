import { Home } from "lucide-react"
import { Button } from "@/components/ui/button"
import { useCommand } from "@/hooks/useCommand"
import { toast } from "@/components/ui/use-toast"

interface HomeButtonProps {
  disabled?: boolean
}

export function HomeButton({ disabled }: HomeButtonProps) {
  const { run, pending } = useCommand()

  const handleHome = () => {
    run("home")
    toast({ title: "Homing started", description: "Tensioning all cables…" })
  }

  return (
    <Button
      onClick={handleHome}
      disabled={disabled || pending}
      variant="outline"
      className="w-full gap-2"
    >
      <Home className="h-4 w-4" />
      {pending ? "Homing…" : "Home / Calibrate"}
    </Button>
  )
}
