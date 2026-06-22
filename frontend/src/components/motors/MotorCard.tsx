import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { cn } from "@/lib/utils"
import type { MotorStatus } from "@/api/types"

interface MotorCardProps {
  motor: MotorStatus
}

export function MotorCard({ motor }: MotorCardProps) {
  const hasFault = motor.fault_code !== 0
  const hasError = Boolean(motor.err)

  return (
    <Card
      className={cn(
        "border-2 transition-colors",
        hasFault || hasError
          ? "border-destructive"
          : "border-transparent",
      )}
    >
      <CardHeader className="pb-2 pt-4 px-4">
        <CardTitle className="text-sm flex items-center justify-between">
          <span>Motor {motor.id}</span>
          <span
            className={cn(
              "h-2.5 w-2.5 rounded-full",
              hasFault || hasError ? "bg-red-500" : "bg-green-500",
            )}
          />
        </CardTitle>
      </CardHeader>
      <CardContent className="px-4 pb-4 space-y-1">
        <Row label="Speed" value={`${motor.speed_rpm} RPM`} />
        <Row label="Torque" value={`${motor.torque_pct}%`} />
        {hasFault && (
          <Row
            label="Fault"
            value={`0x${motor.fault_code.toString(16).toUpperCase()}`}
            danger
          />
        )}
        {hasError && (
          <p className="text-xs text-destructive truncate">{motor.err}</p>
        )}
      </CardContent>
    </Card>
  )
}

function Row({
  label,
  value,
  danger,
}: {
  label: string
  value: string
  danger?: boolean
}) {
  return (
    <div className="flex justify-between text-xs">
      <span className="text-muted-foreground">{label}</span>
      <span className={cn("font-mono", danger && "text-destructive font-semibold")}>
        {value}
      </span>
    </div>
  )
}
