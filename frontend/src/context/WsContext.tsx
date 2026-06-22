import React, {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react"
import { getWsClient } from "@/api/ws"
import type { ConnectionState, SystemStatus } from "@/api/types"

type Client = ReturnType<typeof getWsClient>

interface WsContextShape {
  client: Client
  connectionState: ConnectionState
  status: SystemStatus | null
}

const WsContext = createContext<WsContextShape | null>(null)

export function WsProvider({ children }: { children: React.ReactNode }) {
  const clientRef = useRef<Client | null>(null)
  if (!clientRef.current) clientRef.current = getWsClient()
  const client = clientRef.current

  const [connectionState, setConnectionState] =
    useState<ConnectionState>("disconnected")
  const [status, setStatus] = useState<SystemStatus | null>(null)

  useEffect(() => {
    const unsubConn = client.onConnection(setConnectionState)
    const unsubStatus = client.onStatus(setStatus)
    return () => {
      unsubConn()
      unsubStatus()
    }
  }, [client])

  return (
    <WsContext.Provider value={{ client, connectionState, status }}>
      {children}
    </WsContext.Provider>
  )
}

export function useWsContext(): WsContextShape {
  const ctx = useContext(WsContext)
  if (!ctx) throw new Error("useWsContext must be inside WsProvider")
  return ctx
}
