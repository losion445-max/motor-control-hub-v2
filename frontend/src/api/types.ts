export type ConnectionState = "connecting" | "connected" | "disconnected"

export type EventKind = "progress" | "done" | "error" | "status"

export interface MotorStatus {
  id: number
  speed_rpm: number
  torque_pct: number
  fault_code: number
  err?: string
}

export interface SystemStatus {
  homed: boolean
  x: number
  y: number
  busy: boolean
  motors: MotorStatus[]
}

export interface InCommand {
  id: string
  cmd: string
  x?: number
  y?: number
  speed?: number
  program?: string
  // Debug commands
  motor?: number  // 1-based motor index (1..4)
  rpm?: number    // jog speed
  addr?: number   // FC03 parameter address
  value?: number  // parameter value
}

export interface OutEvent {
  id?: string
  kind: EventKind
  message?: string
  payload?: unknown
}
