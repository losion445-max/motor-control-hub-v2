import { useCallback, useState } from "react"
import { useWsContext } from "@/context/WsContext"
import type { InCommand, OutEvent } from "@/api/types"

interface CommandState {
  pending: boolean
  messages: string[]
  error: string | null
  payload: unknown
}

const initial: CommandState = { pending: false, messages: [], error: null, payload: null }

export function useCommand() {
  const { client } = useWsContext()
  const [state, setState] = useState<CommandState>(initial)

  const run = useCallback(
    (cmd: string, params: Partial<Omit<InCommand, "id" | "cmd">> = {}) => {
      setState({ pending: true, messages: [], error: null, payload: null })
      const id = crypto.randomUUID()

      client.subscribeCmd(id, (ev: OutEvent) => {
        if (ev.kind === "progress") {
          setState((p) => ({
            ...p,
            messages: [...p.messages, ev.message ?? ""],
          }))
        } else if (ev.kind === "done") {
          setState((p) => ({
            ...p,
            pending: false,
            messages: [...p.messages, ev.message ?? ""],
            payload: ev.payload ?? null,
          }))
        } else if (ev.kind === "error") {
          setState({ pending: false, messages: [], error: ev.message ?? "error", payload: null })
        }
      })

      client.send({ id, cmd, ...params })
      return id
    },
    [client],
  )

  const reset = useCallback(() => setState(initial), [])

  return { run, reset, ...state }
}
