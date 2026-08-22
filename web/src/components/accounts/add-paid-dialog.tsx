import { useState } from "react"
import { toast } from "sonner"
import { api } from "@/lib/api"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"

export function AddPaidDialog({ onSaved }: { onSaved: () => void }) {
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState(false)
  const [email, setEmail] = useState("")
  const [apiKey, setApiKey] = useState("")
  const [workspaceId, setWorkspaceId] = useState("")
  const [cookie, setCookie] = useState("")
  const [password, setPassword] = useState("")
  const [recovery, setRecovery] = useState("")

  function openChange(v: boolean) {
    setOpen(v)
    if (v) {
      setEmail("")
      setApiKey("")
      setWorkspaceId("")
      setCookie("")
      setPassword("")
      setRecovery("")
    }
  }

  async function onSubmit() {
    setPending(true)
    try {
      await api.createPaidAccount({
        email,
        api_key: apiKey,
        workspace_id: workspaceId || undefined,
        cookie_header: cookie,
        password: password || undefined,
        recovery_email: recovery || undefined,
        user_id: workspaceId || undefined,
      })
      toast.success("已添加，正在拉用量")
      setOpen(false)
      onSaved()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "添加失败")
    } finally {
      setPending(false)
    }
  }

  const ready = email.trim() && cookie.trim()

  return (
    <Dialog open={open} onOpenChange={openChange}>
      <DialogTrigger asChild>
        <Button variant="outline">手动添加</Button>
      </DialogTrigger>
      <DialogContent className="flex max-h-[85vh] flex-col overflow-hidden sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>添加已付账号</DialogTitle>
        </DialogHeader>
        <div className="grid min-h-0 gap-3 overflow-y-auto">
          <Field label="邮箱" value={email} onChange={setEmail} />
          <Field label="用户 ID（可选）" value={workspaceId} onChange={setWorkspaceId} placeholder="usr-..." />
          <Field label="API Key（可选）" value={apiKey} onChange={setApiKey} />
          <div className="grid gap-2">
            <Label htmlFor="paid-cookie">Cookie</Label>
            <Textarea
              id="paid-cookie"
              rows={4}
              value={cookie}
              onChange={(e) => setCookie(e.target.value)}
              className="max-h-32 font-mono text-xs"
            />
          </div>
          <Field label="密码（可选）" value={password} onChange={setPassword} />
          <Field label="辅助邮箱（可选）" value={recovery} onChange={setRecovery} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            取消
          </Button>
          <Button disabled={pending || !ready} onClick={onSubmit}>
            添加
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Field({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
}) {
  const id = `paid-${label}`
  return (
    <div className="grid gap-2">
      <Label htmlFor={id}>{label}</Label>
      <Input id={id} value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} />
    </div>
  )
}
