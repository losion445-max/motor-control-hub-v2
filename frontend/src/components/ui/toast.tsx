import { X } from "lucide-react"
import { cn } from "@/lib/utils"
import type { Toast } from "./use-toast"

interface ToastItemProps {
  toast: Toast
  onDismiss: (id: string) => void
}

export function ToastItem({ toast, onDismiss }: ToastItemProps) {
  return (
    <div
      className={cn(
        "pointer-events-auto flex w-full items-center justify-between space-x-4 overflow-hidden rounded-md border p-4 shadow-lg transition-all",
        toast.variant === "destructive"
          ? "border-destructive bg-destructive text-destructive-foreground"
          : "bg-background text-foreground border-border",
      )}
    >
      <div className="flex-1 space-y-1">
        <p className="text-sm font-semibold">{toast.title}</p>
        {toast.description && (
          <p className="text-sm opacity-90">{toast.description}</p>
        )}
      </div>
      <button
        onClick={() => onDismiss(toast.id)}
        className="rounded-md p-1 opacity-70 hover:opacity-100"
      >
        <X className="h-4 w-4" />
      </button>
    </div>
  )
}
