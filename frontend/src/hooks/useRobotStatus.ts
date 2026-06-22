import { useWsContext } from "@/context/WsContext"
import type { SystemStatus } from "@/api/types"

export function useRobotStatus(): SystemStatus | null {
  return useWsContext().status
}
