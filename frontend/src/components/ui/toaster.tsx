import { useEffect } from "react"
import { createPortal } from "react-dom"
import { ToastItem } from "./toast"
import { setToastDispatch, useToastState } from "./use-toast"

export function Toaster() {
  const { toasts, addToast, dismiss } = useToastState()

  useEffect(() => {
    setToastDispatch(addToast)
    return () => setToastDispatch(() => {})
  }, [addToast])

  return createPortal(
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 w-80">
      {toasts.map((t) => (
        <ToastItem key={t.id} toast={t} onDismiss={dismiss} />
      ))}
    </div>,
    document.body,
  )
}
