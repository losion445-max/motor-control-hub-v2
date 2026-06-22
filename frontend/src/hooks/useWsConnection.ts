import { useCallback } from "react"
import { useWsContext } from "@/context/WsContext"
import { getWsClient } from "@/api/ws"
import type { ConnectionState } from "@/api/types"

interface WsConnectionHook {
  connectionState: ConnectionState
  url: string
  reconnect: (url: string) => void
}

export function useWsConnection(): WsConnectionHook {
  const { connectionState } = useWsContext()
  const client = getWsClient()

  const reconnect = useCallback(
    (url: string) => {
      localStorage.setItem("ws_url", url)
      client.connect(url)
    },
    [client],
  )

  return { connectionState, url: client.getUrl(), reconnect }
}
