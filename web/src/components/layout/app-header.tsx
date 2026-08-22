import { NavLink, useLocation } from "react-router-dom"
import { Bot, House, Menu, ScrollText, Settings, Users, Zap } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet"
import { ThemeToggle } from "@/components/layout/theme-toggle"
import { cn } from "@/lib/utils"

const items = [
  { title: "仪表盘", url: "/", icon: House },
  { title: "账号", url: "/account", icon: Users },
  { title: "日志", url: "/logs", icon: ScrollText },
  { title: "提取支付链接", url: "/automation", icon: Bot },
  { title: "设置", url: "/settings", icon: Settings },
]

function isActive(pathname: string, url: string) {
  return url === "/" ? pathname === "/" : pathname.startsWith(url)
}

function NavItems({ onSelect, stacked }: { onSelect?: () => void; stacked?: boolean }) {
  const location = useLocation()

  return (
    <nav className={cn("flex items-center gap-1", stacked && "flex-col items-stretch")}>
      {items.map((item) => {
        const active = isActive(location.pathname, item.url)
        return (
          <NavLink
            key={item.url}
            to={item.url}
            onClick={onSelect}
            className={cn(
              "inline-flex h-8 items-center gap-1.5 rounded-md px-3 text-sm font-medium transition-colors",
              stacked && "h-10 w-full justify-start",
              active
                ? "bg-background text-foreground shadow-sm ring-1 ring-foreground/10"
                : "text-muted-foreground hover:bg-background/60 hover:text-foreground"
            )}
          >
            <item.icon className="size-4" />
            {item.title}
          </NavLink>
        )
      })}
    </nav>
  )
}

export function AppHeader() {
  return (
    <header className="sticky top-0 z-40 border-b bg-card/95 shadow-sm backdrop-blur supports-backdrop-filter:bg-card/80">
      <div className="mx-auto flex h-14 max-w-[96rem] items-center gap-3 px-4">
        <NavLink to="/" className="flex items-center gap-2 font-semibold">
          <span className="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-sm">
            <Zap className="size-4" />
          </span>
          <span className="hidden sm:inline">ClinePass</span>
        </NavLink>

        <div className="hidden rounded-lg bg-muted p-1 md:flex">
          <NavItems />
        </div>

        <div className="ml-auto flex items-center gap-1">
          <ThemeToggle />
          <Sheet>
            <SheetTrigger asChild>
              <Button variant="outline" size="icon" className="md:hidden">
                <Menu />
              </Button>
            </SheetTrigger>
            <SheetContent side="top">
              <SheetHeader>
                <SheetTitle>导航</SheetTitle>
              </SheetHeader>
              <div className="px-4 pb-4">
                <NavItems stacked />
              </div>
            </SheetContent>
          </Sheet>
        </div>
      </div>
    </header>
  )
}
