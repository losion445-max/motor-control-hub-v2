import { useRef } from "react"

const W = 1400
const H = 2400

const CABLE_COLORS = ["#ef4444", "#3b82f6", "#22c55e", "#f59e0b"]

const ANCHORS = [
  { x: 0, y: 0, label: "M1" },
  { x: W, y: 0, label: "M2" },
  { x: W, y: H, label: "M3" },
  { x: 0, y: H, label: "M4" },
]

interface WorkspaceViewProps {
  x: number
  y: number
  homed: boolean
  onTarget?: (x: number, y: number) => void
  targetPos?: { x: number; y: number } | null
}

export function WorkspaceView({
  x,
  y,
  homed,
  onTarget,
  targetPos,
}: WorkspaceViewProps) {
  const svgRef = useRef<SVGSVGElement>(null)

  const handleClick = (e: React.MouseEvent<SVGSVGElement>) => {
    if (!onTarget || !svgRef.current) return
    const rect = svgRef.current.getBoundingClientRect()
    const scaleX = W / rect.width
    const scaleY = H / rect.height
    const rx = Math.min(W, Math.max(0, (e.clientX - rect.left) * scaleX))
    const ry = Math.min(H, Math.max(0, (e.clientY - rect.top) * scaleY))
    onTarget(Math.round(rx), Math.round(ry))
  }

  const gridLines: React.ReactNode[] = []
  for (let gx = 200; gx < W; gx += 200) {
    gridLines.push(
      <line
        key={`vg${gx}`}
        x1={gx}
        y1={0}
        x2={gx}
        y2={H}
        stroke="#e5e7eb"
        strokeWidth={2}
      />,
    )
  }
  for (let gy = 200; gy < H; gy += 200) {
    gridLines.push(
      <line
        key={`hg${gy}`}
        x1={0}
        y1={gy}
        x2={W}
        y2={gy}
        stroke="#e5e7eb"
        strokeWidth={2}
      />,
    )
  }

  return (
    <div
      className="w-full"
      style={{ aspectRatio: `${W}/${H}`, maxHeight: "100%" }}
    >
    <svg
      ref={svgRef}
      viewBox={`0 0 ${W} ${H}`}
      width="100%"
      height="100%"
      preserveAspectRatio="xMidYMid meet"
      className="cursor-crosshair rounded border border-border bg-slate-50 block"
      onClick={handleClick}
    >
      <rect x={0} y={0} width={W} height={H} fill="#f8fafc" />
      {gridLines}

      {ANCHORS.map((a, i) =>
        homed ? (
          <line
            key={`cable${i}`}
            x1={a.x}
            y1={a.y}
            x2={x}
            y2={y}
            stroke={CABLE_COLORS[i]}
            strokeWidth={3}
            opacity={0.6}
          />
        ) : null,
      )}

      {ANCHORS.map((a) => (
        <g key={a.label}>
          <circle cx={a.x} cy={a.y} r={20} fill="#1e293b" />
          <text
            x={a.x + (a.x === 0 ? 25 : -25)}
            y={a.y + (a.y === 0 ? 30 : -15)}
            fill="#1e293b"
            fontSize={60}
            fontWeight="bold"
            textAnchor={a.x === 0 ? "start" : "end"}
          >
            {a.label}
          </text>
        </g>
      ))}

      {targetPos && (
        <g>
          <circle
            cx={targetPos.x}
            cy={targetPos.y}
            r={30}
            fill="none"
            stroke="#6366f1"
            strokeWidth={3}
            strokeDasharray="10 5"
          />
          <line
            x1={targetPos.x - 50}
            y1={targetPos.y}
            x2={targetPos.x + 50}
            y2={targetPos.y}
            stroke="#6366f1"
            strokeWidth={2}
            strokeDasharray="6 4"
          />
          <line
            x1={targetPos.x}
            y1={targetPos.y - 50}
            x2={targetPos.x}
            y2={targetPos.y + 50}
            stroke="#6366f1"
            strokeWidth={2}
            strokeDasharray="6 4"
          />
        </g>
      )}

      <g>
        <circle
          cx={x}
          cy={y}
          r={25}
          fill={homed ? "#3b82f6" : "#94a3b8"}
          stroke="#fff"
          strokeWidth={4}
        />
        <line
          x1={x - 45}
          y1={y}
          x2={x + 45}
          y2={y}
          stroke="#fff"
          strokeWidth={2}
        />
        <line
          x1={x}
          y1={y - 45}
          x2={x}
          y2={y + 45}
          stroke="#fff"
          strokeWidth={2}
        />
      </g>
    </svg>
    </div>
  )
}
