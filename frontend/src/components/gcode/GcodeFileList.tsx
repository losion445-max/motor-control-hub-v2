import { Trash2, FileCode } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

export interface GcodeFile {
  name: string
  content: string
  savedAt: string
}

const STORAGE_KEY = "gcode_files"
const MAX_FILES = 20

export function loadGcodeFiles(): GcodeFile[] {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "[]") as GcodeFile[]
  } catch {
    return []
  }
}

export function saveGcodeFile(name: string, content: string) {
  const files = loadGcodeFiles().filter((f) => f.name !== name)
  const updated: GcodeFile[] = [
    { name, content, savedAt: new Date().toISOString() },
    ...files,
  ].slice(0, MAX_FILES)
  localStorage.setItem(STORAGE_KEY, JSON.stringify(updated))
  return updated
}

export function deleteGcodeFile(name: string): GcodeFile[] {
  const updated = loadGcodeFiles().filter((f) => f.name !== name)
  localStorage.setItem(STORAGE_KEY, JSON.stringify(updated))
  return updated
}

interface GcodeFileListProps {
  files: GcodeFile[]
  selected: string | null
  onSelect: (file: GcodeFile) => void
  onDelete: (name: string) => void
}

export function GcodeFileList({
  files,
  selected,
  onSelect,
  onDelete,
}: GcodeFileListProps) {
  if (files.length === 0) {
    return (
      <p className="text-sm text-muted-foreground text-center py-4">
        No files saved
      </p>
    )
  }

  return (
    <div className="space-y-1">
      {files.map((f) => (
        <div
          key={f.name}
          className={cn(
            "flex items-center gap-2 rounded px-2 py-1.5 cursor-pointer hover:bg-accent transition-colors",
            selected === f.name && "bg-accent",
          )}
          onClick={() => onSelect(f)}
        >
          <FileCode className="h-4 w-4 shrink-0 text-muted-foreground" />
          <div className="flex-1 min-w-0">
            <p className="text-sm truncate">{f.name}</p>
            <p className="text-xs text-muted-foreground">
              {new Date(f.savedAt).toLocaleDateString()}
            </p>
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6 shrink-0"
            onClick={(e) => {
              e.stopPropagation()
              onDelete(f.name)
            }}
          >
            <Trash2 className="h-3 w-3" />
          </Button>
        </div>
      ))}
    </div>
  )
}
