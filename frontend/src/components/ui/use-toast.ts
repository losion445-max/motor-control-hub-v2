import { useState, useCallback } from "react"

export type ToastVariant = "default" | "destructive"

export interface Toast {
  id: string
  title: string
  description?: string
  variant?: ToastVariant
}

type ToastInput = Omit<Toast, "id">

let _dispatch: ((t: Toast) => void) | null = null

export function setToastDispatch(fn: (t: Toast) => void) {
  _dispatch = fn
}

export function toast(input: ToastInput) {
  const t: Toast = { id: crypto.randomUUID(), ...input }
  _dispatch?.(t)
}

export function useToastState() {
  const [toasts, setToasts] = useState<Toast[]>([])

  const addToast = useCallback((t: Toast) => {
    setToasts((prev) => [...prev, t])
    setTimeout(() => {
      setToasts((prev) => prev.filter((x) => x.id !== t.id))
    }, 4000)
  }, [])

  const dismiss = useCallback((id: string) => {
    setToasts((prev) => prev.filter((x) => x.id !== id))
  }, [])

  return { toasts, addToast, dismiss }
}
