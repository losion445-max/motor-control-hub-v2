import { useEffect, useRef } from "react"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"

interface GcodeViewerProps {
  content: string
  currentLine?: number
}

export function GcodeViewer({ content, currentLine }: GcodeViewerProps) {
  const lineRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    lineRef.current?.scrollIntoView({ block: "center", behavior: "smooth" })
  }, [currentLine])

  if (!content) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-muted-foreground">
        No file selected
      </div>
    )
  }

  const lines = content.split("\n")

  return (
    <ScrollArea className="h-full font-mono text-xs">
      <div className="p-2">
        {lines.map((line, i) => (
          <div
            key={i}
            ref={currentLine === i ? lineRef : undefined}
            className={cn(
              "flex gap-3 px-2 py-0.5 rounded leading-5",
              currentLine === i && "bg-primary text-primary-foreground",
            )}
          >
            <span className="select-none w-8 text-right text-muted-foreground shrink-0">
              {i + 1}
            </span>
            <span className="whitespace-pre">{line}</span>
          </div>
        ))}
      </div>
    </ScrollArea>
  )
}
