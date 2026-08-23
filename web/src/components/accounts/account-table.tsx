import { MoreHorizontal } from "lucide-react"
import type { Account } from "@/lib/types"
import { statusLabel, statusVariant } from "@/lib/status"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { loginProviderLabel, normalizeLoginProvider } from "@/lib/login-provider"

export function AccountTable({
  accounts,
  onLogin,
  onDetail,
  onRemove,
  onRefresh,
  currentSteps,
  selectedId,
  onSelect,
}: {
  accounts: Account[]
  onLogin: (account: Account) => void
  onDetail: (account: Account) => void
  onRemove: (account: Account) => void
  onRefresh?: (account: Account) => void
  currentSteps?: Record<string, string>
  selectedId?: string
  onSelect?: (account: Account) => void
}) {
  return (
    <div className="overflow-hidden rounded-xl border bg-card shadow-sm">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>邮箱</TableHead>
            <TableHead>登录</TableHead>
            <TableHead>状态</TableHead>
            <TableHead>当前步骤</TableHead>
            <TableHead>支付链接</TableHead>
            <TableHead className="w-12" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {accounts.length === 0 ? (
            <TableRow>
              <TableCell colSpan={6} className="h-24 text-center text-muted-foreground">
                这批还没有账号。
              </TableCell>
            </TableRow>
          ) : (
            accounts.map((a) => (
              <TableRow
                key={a.id}
                data-state={selectedId === a.id ? "selected" : undefined}
                className={onSelect ? "cursor-pointer" : undefined}
                onClick={() => onSelect?.(a)}
              >
                <TableCell className="max-w-[18rem] whitespace-normal">
                  <div className="truncate font-medium">{a.email}</div>
                  <div className="truncate text-xs text-muted-foreground">{a.recovery_email || "无辅助邮箱"}</div>
                  {a.last_error ? (
                    <div className="mt-1 truncate text-xs text-destructive" title={a.last_error}>
                      {a.last_error}
                    </div>
                  ) : null}
                </TableCell>
                <TableCell>
                  <Badge variant={normalizeLoginProvider(a.login_provider) === "microsoft" ? "secondary" : "outline"}>
                    {loginProviderLabel(a.login_provider)}
                  </Badge>
                </TableCell>
                <TableCell>
                  <div className="flex flex-wrap items-center gap-1">
                    {a.paid_at ? (
                      <Badge className="bg-violet-600 text-white hover:bg-violet-600">已支付</Badge>
                    ) : (
                      <Badge variant={statusVariant(a.status)}>{statusLabel(a.status)}</Badge>
                    )}
                  </div>
                </TableCell>
                <TableCell className="max-w-[16rem] truncate text-xs text-muted-foreground" title={currentSteps?.[a.id] || ""}>
                  {currentSteps?.[a.id] || "—"}
                </TableCell>
                <TableCell className="max-w-[14rem] truncate text-xs text-muted-foreground" title={a.payment_url || ""}>
                  {a.payment_url ? "已生成" : "还没有"}
                </TableCell>
                <TableCell onClick={(e) => e.stopPropagation()}>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon">
                        <MoreHorizontal />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      {a.paid_at ? (
                        <DropdownMenuItem disabled>已支付，不再提取链接</DropdownMenuItem>
                      ) : (
                        <>
                          <DropdownMenuItem onClick={() => onLogin(a)}>重新登录并生成链接</DropdownMenuItem>
                          {onRefresh && (a.has_cookies || a.cookie_header) ? (
                            <DropdownMenuItem onClick={() => onRefresh(a)}>刷新过期的支付链接</DropdownMenuItem>
                          ) : null}
                        </>
                      )}
                      <DropdownMenuItem onClick={() => onDetail(a)}>详情</DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive" onClick={() => onRemove(a)}>
                        删除
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  )
}
