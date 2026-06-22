import { MotorCard } from "./MotorCard"
import type { MotorStatus } from "@/api/types"

interface MotorGridProps {
  motors: MotorStatus[]
}

export function MotorGrid({ motors }: MotorGridProps) {
  return (
    <div className="grid grid-cols-2 gap-2">
      {motors.map((m) => (
        <MotorCard key={m.id} motor={m} />
      ))}
    </div>
  )
}
