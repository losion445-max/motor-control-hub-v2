import { useState } from "react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"
import { GcodeUpload } from "@/components/gcode/GcodeUpload"
import {
  GcodeFileList,
  loadGcodeFiles,
  saveGcodeFile,
  deleteGcodeFile,
} from "@/components/gcode/GcodeFileList"
import type { GcodeFile } from "@/components/gcode/GcodeFileList"
import { GcodeViewer } from "@/components/gcode/GcodeViewer"
import { GcodeControls } from "@/components/gcode/GcodeControls"
import { useRobotStatus } from "@/hooks/useRobotStatus"

export function GcodePage() {
  const status = useRobotStatus()
  const busy = status?.busy ?? false
  const homed = status?.homed ?? false

  const [files, setFiles] = useState<GcodeFile[]>(() => loadGcodeFiles())
  const [selected, setSelected] = useState<GcodeFile | null>(null)
  const [speedOverride, setSpeedOverride] = useState("")

  const handleUpload = (name: string, content: string) => {
    const updated = saveGcodeFile(name, content)
    setFiles(updated)
    setSelected({ name, content, savedAt: new Date().toISOString() })
  }

  const handleSelect = (file: GcodeFile) => {
    setSelected(file)
  }

  const handleDelete = (name: string) => {
    const updated = deleteGcodeFile(name)
    setFiles(updated)
    if (selected?.name === name) setSelected(null)
  }

  return (
    <div className="grid grid-cols-[220px_1fr] gap-4 p-4 h-full">
      <div className="flex flex-col gap-3 overflow-y-auto">
        <GcodeUpload onFile={handleUpload} />
        <Separator />
        <GcodeFileList
          files={files}
          selected={selected?.name ?? null}
          onSelect={handleSelect}
          onDelete={handleDelete}
        />
      </div>

      <div className="flex flex-col gap-3 min-h-0">
        <Card className="flex-1 flex flex-col min-h-0">
          <CardHeader className="pb-2 pt-4 px-4 shrink-0">
            <CardTitle className="text-sm">
              {selected ? selected.name : "No file selected"}
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-2 flex-1 min-h-0">
            <GcodeViewer content={selected?.content ?? ""} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2 pt-4 px-4">
            <CardTitle className="text-sm">Execute</CardTitle>
          </CardHeader>
          <CardContent className="px-4 pb-4">
            <GcodeControls
              content={selected?.content ?? ""}
              disabled={busy || !homed}
              speedOverride={speedOverride}
              onSpeedOverrideChange={setSpeedOverride}
            />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
