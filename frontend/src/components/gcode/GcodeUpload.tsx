import { useRef, useState } from "react"
import { Upload } from "lucide-react"
import { cn } from "@/lib/utils"

interface GcodeUploadProps {
  onFile: (name: string, content: string) => void
}

export function GcodeUpload({ onFile }: GcodeUploadProps) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [dragging, setDragging] = useState(false)

  const handleFile = (file: File) => {
    const reader = new FileReader()
    reader.onload = (e) => {
      if (typeof e.target?.result === "string") {
        onFile(file.name, e.target.result)
      }
    }
    reader.readAsText(file)
  }

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    setDragging(false)
    const file = e.dataTransfer.files[0]
    if (file) handleFile(file)
  }

  const handleInput = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (file) handleFile(file)
    e.target.value = ""
  }

  return (
    <div
      onClick={() => inputRef.current?.click()}
      onDrop={handleDrop}
      onDragOver={(e) => { e.preventDefault(); setDragging(true) }}
      onDragLeave={() => setDragging(false)}
      className={cn(
        "flex flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed p-6 cursor-pointer transition-colors",
        dragging
          ? "border-primary bg-accent"
          : "border-border hover:border-primary hover:bg-accent/50",
      )}
    >
      <Upload className="h-6 w-6 text-muted-foreground" />
      <p className="text-sm text-muted-foreground text-center">
        Drop .gcode / .nc / .txt or click to browse
      </p>
      <input
        ref={inputRef}
        type="file"
        accept=".gcode,.nc,.txt"
        className="hidden"
        onChange={handleInput}
      />
    </div>
  )
}
