import { useState } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"
import { WorkspaceView } from "@/components/control/WorkspaceView"
import { MovePanel } from "@/components/control/MovePanel"
import { JogPad } from "@/components/control/JogPad"
import { HomeButton } from "@/components/control/HomeButton"
import { HoldTensionButton } from "@/components/control/HoldTensionButton"
import { SetHomeButton } from "@/components/control/SetHomeButton"
import { StopButton } from "@/components/control/StopButton"
import { MotorGrid } from "@/components/motors/MotorGrid"
import { useRobotStatus } from "@/hooks/useRobotStatus"

interface ControlPageProps {
  rapidSpeed: number
  feedSpeed: number
}

const PLACEHOLDER_MOTORS = [
  { id: 1, speed_rpm: 0, torque_pct: 0, fault_code: 0 },
  { id: 2, speed_rpm: 0, torque_pct: 0, fault_code: 0 },
  { id: 3, speed_rpm: 0, torque_pct: 0, fault_code: 0 },
  { id: 4, speed_rpm: 0, torque_pct: 0, fault_code: 0 },
]

export function ControlPage({ rapidSpeed, feedSpeed }: ControlPageProps) {
  const status = useRobotStatus()
  const homed = status?.homed ?? false
  const busy = status?.busy ?? false
  const posX = status?.x ?? 700
  const posY = status?.y ?? 1200
  const motors = status?.motors ?? PLACEHOLDER_MOTORS

  const [targetX, setTargetX] = useState<{ x: string; y: string } | null>(null)
  const [mvX, setMvX] = useState("700")
  const [mvY, setMvY] = useState("1200")
  const [mvSpeed, setMvSpeed] = useState("")

  const handleWorkspaceClick = (x: number, y: number) => {
    setMvX(String(x))
    setMvY(String(y))
    setTargetX({ x: String(x), y: String(y) })
  }

  const targetPos = targetX
    ? { x: Number(targetX.x), y: Number(targetX.y) }
    : null

  return (
    <div className="grid grid-cols-[1fr_320px] gap-4 p-4 h-full">
      <div className="flex flex-col gap-4">
        <WorkspaceView
          x={posX}
          y={posY}
          homed={homed}
          onTarget={handleWorkspaceClick}
          targetPos={targetPos}
        />
      </div>

      <div className="flex flex-col gap-3 overflow-y-auto">
        <StopButton />

        <Card>
          <CardHeader className="pb-2 pt-4 px-4">
            <CardTitle className="text-sm">System</CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4 flex flex-col gap-2">
            <HomeButton disabled={busy} />
            <SetHomeButton disabled={busy} />
            <HoldTensionButton disabled={busy || !homed} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2 pt-4 px-4">
            <CardTitle className="text-sm">Move to Point</CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4">
            <MovePanel
              disabled={busy || !homed}
              x={mvX}
              y={mvY}
              speed={mvSpeed}
              onXChange={setMvX}
              onYChange={setMvY}
              onSpeedChange={setMvSpeed}
              rapidSpeed={rapidSpeed}
              feedSpeed={feedSpeed}
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2 pt-4 px-4">
            <CardTitle className="text-sm">Jog</CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4">
            <JogPad
              currentX={posX}
              currentY={posY}
              feedSpeed={feedSpeed}
              disabled={busy}
            />
          </CardContent>
        </Card>

        <Separator />

        <Card>
          <CardHeader className="pb-2 pt-4 px-4">
            <CardTitle className="text-sm">Motors</CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4">
            <MotorGrid motors={motors} />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
