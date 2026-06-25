import { useState, useEffect } from "react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useWsContext } from "@/context/WsContext"
import { useCommand } from "@/hooks/useCommand"
import { toast } from "@/components/ui/use-toast"

// ── FC03 parameter definitions ────────────────────────────────────────────────

interface ParamDef {
  addr: number
  name: string
  unit: string
  description: string
  writable: boolean
  defaultVal?: number
}

const PARAMS: ParamDef[] = [
  { addr: 4,   name: "Control Mode",       unit: "",        description: "0=position, 1=speed, 2=torque (P-004)", writable: false },
  { addr: 25,  name: "Speed Source",       unit: "",        description: "0=analog, 1=internal multi-speed (P-025)", writable: false },
  { addr: 60,  name: "Accel Time",         unit: "ms/kRPM", description: "Acceleration ramp (P-060)", writable: true, defaultVal: 100 },
  { addr: 61,  name: "Decel Time",         unit: "ms/kRPM", description: "Deceleration ramp (P-061)", writable: true, defaultVal: 100 },
  { addr: 69,  name: "Torque Limit (fwd)", unit: "%",       description: "Forward torque cap 0..300% (P-069)", writable: true, defaultVal: 300 },
  { addr: 70,  name: "Torque Limit (rev)", unit: "%",       description: "Reverse torque cap 0..300% (P-070)", writable: true, defaultVal: 300 },
  { addr: 75,  name: "Max Speed",          unit: "RPM",     description: "Maximum speed limit (P-075)", writable: true, defaultVal: 3000 },
  { addr: 98,  name: "Servo-On Mode",      unit: "",        description: "0=ext SON pin, 1=always ON (P-098)", writable: false },
  { addr: 137, name: "Speed Preset 1",     unit: "RPM",     description: "Internal speed preset 1 — current motion command (P-137)", writable: true },
  { addr: 172, name: "Encoder Lines",      unit: "lines",   description: "Encoder lines/rev (×4=PPR); 2500→10000 ppr (P-172)", writable: false },
  { addr: 181, name: "Slave ID",           unit: "",        description: "Modbus slave address (P-181)", writable: false },
  { addr: 182, name: "Baud Rate",          unit: "",        description: "0=4800…5=115200 baud (P-182)", writable: false },
  { addr: 183, name: "Data Format",        unit: "",        description: "0=8N1, 1=8E1, 2=8O1… (P-183)", writable: false },
]

// ── Status register viewer (FC04) ─────────────────────────────────────────────

interface MotorStatusFields {
  speed_rpm?: number
  torque_pct?: number
  torque_ref_pct?: number
  current_a10?: number
  speed_ref_rpm?: number
  fault_code?: number
  position32?: number
  heatsink_temp_c?: number
  module_temp_c?: number
  bus_voltage_v?: number
}

const STATUS_ROWS: { key: keyof MotorStatusFields; label: string; addr: string; unit: string; description: string }[] = [
  { key: "speed_rpm",       addr: "0x00", label: "Speed",           unit: "RPM",  description: "Current motor speed" },
  { key: "torque_pct",      addr: "0x09", label: "Torque",          unit: "%",    description: "Instantaneous torque (signed, % of 2.4 Nm)" },
  { key: "torque_ref_pct",  addr: "0x0F", label: "Torque Setpoint", unit: "%",    description: "Active torque command" },
  { key: "current_a10",     addr: "0x0B", label: "Current",         unit: "A×10", description: "Phase current (divide by 10 for Amps)" },
  { key: "speed_ref_rpm",   addr: "0x0E", label: "Speed Setpoint",  unit: "RPM",  description: "Active internal speed command" },
  { key: "fault_code",      addr: "0x1A", label: "Fault Code",      unit: "",     description: "0 = no fault; non-zero = active fault" },
  { key: "position32",      addr: "0x1F/20", label: "Abs Position", unit: "pulses", description: "32-bit absolute encoder counter" },
  { key: "heatsink_temp_c", addr: "0x26", label: "Heatsink Temp",   unit: "°C",   description: "Drive heatsink temperature" },
  { key: "module_temp_c",   addr: "0x27", label: "Module Temp",     unit: "°C",   description: "Power module temperature" },
  { key: "bus_voltage_v",   addr: "0x28", label: "Bus Voltage",     unit: "V",    description: "DC bus voltage (nominal ≈ 310 V)" },
]

function StatusTable({ motor }: { motor: number }) {
  const { run, pending, payload } = useCommand()
  const [data, setData] = useState<MotorStatusFields | null>(null)

  useEffect(() => {
    if (payload && typeof payload === "object") {
      setData(payload as MotorStatusFields)
    }
  }, [payload])

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <span className="text-xs font-medium">FC04 Status Registers (read-only)</span>
        <Button
          size="sm"
          variant="outline"
          className="h-6 text-xs ml-auto"
          onClick={() => run("read_motor_status", { motor })}
          disabled={pending}
        >
          {pending ? "Reading…" : "Read All"}
        </Button>
      </div>
      <div className="overflow-x-auto rounded border">
        <table className="w-full text-xs border-collapse">
          <thead>
            <tr className="bg-muted/50 border-b">
              <th className="text-left py-1.5 px-3 text-muted-foreground font-medium w-16">Addr</th>
              <th className="text-left py-1.5 px-3 text-muted-foreground font-medium">Name</th>
              <th className="text-left py-1.5 px-3 text-muted-foreground font-medium w-24">Value</th>
              <th className="text-left py-1.5 px-3 text-muted-foreground font-medium hidden sm:table-cell">Description</th>
            </tr>
          </thead>
          <tbody>
            {STATUS_ROWS.map((row) => {
              const val = data?.[row.key]
              const faultHighlight = row.key === "fault_code" && val !== undefined && val !== 0
              return (
                <tr key={row.key} className={`border-b last:border-0 hover:bg-muted/20 ${faultHighlight ? "bg-destructive/10" : ""}`}>
                  <td className="py-1 px-3 font-mono text-muted-foreground text-[11px]">{row.addr}</td>
                  <td className="py-1 px-3 font-medium">{row.label}</td>
                  <td className="py-1 px-3 font-mono">
                    {val !== undefined ? (
                      <span className={faultHighlight ? "text-destructive font-bold" : ""}>
                        {val} {row.unit}
                      </span>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </td>
                  <td className="py-1 px-3 text-muted-foreground hidden sm:table-cell">{row.description}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// ── Single parameter row ──────────────────────────────────────────────────────

function ParamRow({ param, motor }: { param: ParamDef; motor: number }) {
  const { client } = useWsContext()
  const { run: readRun, pending: readPending, payload: readPayload } = useCommand()
  const [readVal, setReadVal] = useState<number | null>(null)
  const [writeVal, setWriteVal] = useState(param.defaultVal?.toString() ?? "")

  useEffect(() => {
    if (readPayload && typeof readPayload === "object") {
      const p = readPayload as { value?: number }
      if (p.value !== undefined) setReadVal(p.value)
    }
  }, [readPayload])

  const write = () => {
    const v = Number(writeVal)
    if (isNaN(v) || !Number.isInteger(v)) {
      toast({ title: "Введите целое число", variant: "destructive" })
      return
    }
    client.sendNow({
      id: crypto.randomUUID(),
      cmd: "write_param",
      motor,
      addr: param.addr,
      value: v,
    })
    toast({ title: `M${motor} P-${String(param.addr).padStart(3, "0")} = ${v} (RAM)` })
  }

  return (
    <tr className="border-b last:border-0 hover:bg-muted/20">
      <td className="py-1 px-3 font-mono text-muted-foreground text-[11px]">
        P-{String(param.addr).padStart(3, "0")}
      </td>
      <td className="py-1 px-3 font-medium whitespace-nowrap">{param.name}</td>
      <td className="py-1 px-3 text-muted-foreground text-[11px]">{param.unit || "—"}</td>
      <td className="py-1 px-3">
        <div className="flex items-center gap-1.5">
          <span className="font-mono text-sm min-w-[40px]">
            {readVal !== null ? readVal : <span className="text-muted-foreground text-xs">?</span>}
          </span>
          <Button
            size="sm"
            variant="ghost"
            className="h-5 text-xs px-1.5"
            onClick={() => readRun("read_param", { motor, addr: param.addr })}
            disabled={readPending}
          >
            {readPending ? "…" : "R"}
          </Button>
        </div>
      </td>
      <td className="py-1 px-3">
        {param.writable ? (
          <div className="flex items-center gap-1">
            <Input
              type="number"
              value={writeVal}
              onChange={(e) => setWriteVal(e.target.value)}
              className="h-6 w-20 text-xs"
            />
            <Button size="sm" variant="outline" className="h-6 text-xs px-2" onClick={write}>
              W
            </Button>
          </div>
        ) : (
          <span className="text-xs text-muted-foreground">read-only</span>
        )}
      </td>
      <td className="py-1 px-3 text-muted-foreground text-[11px] hidden lg:table-cell">{param.description}</td>
    </tr>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export function RegisterViewer() {
  const [motor, setMotor] = useState(1)

  return (
    <div className="space-y-5">
      <div className="flex items-center gap-3 flex-wrap">
        <Label className="text-sm font-medium shrink-0">Мотор</Label>
        <div className="flex gap-1">
          {([1, 2, 3, 4] as const).map((m) => (
            <Button
              key={m}
              size="sm"
              variant={motor === m ? "default" : "outline"}
              className="h-7 w-10 text-xs"
              onClick={() => setMotor(m)}
            >
              M{m}
            </Button>
          ))}
        </div>
        <span className="text-xs text-muted-foreground">
          Slave ID {motor} — {["top-left (M1)", "top-right (M2)", "bot-right (M3)", "bot-left (M4)"][motor - 1]}
        </span>
      </div>

      <StatusTable motor={motor} />

      <div className="space-y-2">
        <span className="text-xs font-medium">FC03 Parameters — R = прочитать, W = записать в RAM</span>
        <div className="overflow-x-auto rounded border">
          <table className="w-full text-xs border-collapse">
            <thead>
              <tr className="bg-muted/50 border-b">
                <th className="text-left py-1.5 px-3 text-muted-foreground font-medium w-16">Param</th>
                <th className="text-left py-1.5 px-3 text-muted-foreground font-medium">Name</th>
                <th className="text-left py-1.5 px-3 text-muted-foreground font-medium w-14">Unit</th>
                <th className="text-left py-1.5 px-3 text-muted-foreground font-medium w-28">Value</th>
                <th className="text-left py-1.5 px-3 text-muted-foreground font-medium w-36">Write</th>
                <th className="text-left py-1.5 px-3 text-muted-foreground font-medium hidden lg:table-cell">Description</th>
              </tr>
            </thead>
            <tbody>
              {PARAMS.map((p) => (
                <ParamRow key={p.addr} param={p} motor={motor} />
              ))}
            </tbody>
          </table>
        </div>
        <p className="text-xs text-muted-foreground bg-amber-50 border border-amber-200 rounded p-2">
          ⚠ Запись изменяет только RAM-значение привода. При перезагрузке сбросится в EEPROM-значение.
          Для сохранения нужна команда FC41 (SaveEEPROM) — не доступна через UI.
        </p>
      </div>
    </div>
  )
}
