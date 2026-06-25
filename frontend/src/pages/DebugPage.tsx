import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"
import { MotorJog } from "@/components/debug/MotorJog"
import { RegisterViewer } from "@/components/debug/RegisterViewer"

export function DebugPage() {
  return (
    <div className="p-4 space-y-4 overflow-y-auto h-full">
      <Card>
        <CardHeader className="pb-2 pt-4 px-4">
          <CardTitle className="text-sm">Manual Motor Jog</CardTitle>
        </CardHeader>
        <CardContent className="px-4 pb-4">
          <MotorJog />
        </CardContent>
      </Card>

      <Separator />

      <Card>
        <CardHeader className="pb-2 pt-4 px-4">
          <CardTitle className="text-sm">Register Viewer / Parameter Editor</CardTitle>
        </CardHeader>
        <CardContent className="px-4 pb-4">
          <RegisterViewer />
        </CardContent>
      </Card>
    </div>
  )
}
