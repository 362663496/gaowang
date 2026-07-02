"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import {
  ArrowLeftRight,
  BarChart3,
  Boxes,
  DatabaseBackup,
  LayoutDashboard,
  LogOut,
  Menu,
  Package,
  Settings,
  Store,
  Users,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { apiDeleteSession, devSessionEvent, readDevSession, type Role } from "@/lib/api";
import { cn } from "@/lib/utils";

const navItems = [
  { label: "仪表盘", href: "/dashboard", icon: LayoutDashboard },
  { label: "商品", href: "/products", icon: Package },
  { label: "店铺", href: "/shops", icon: Store },
  { label: "当前库存", href: "/inventory", icon: Boxes },
  { label: "流水记录", href: "/stock-movements", icon: ArrowLeftRight },
  { label: "报表", href: "/reports", icon: BarChart3 },
  { label: "用户管理", href: "/users", icon: Users, roles: ["admin"] },
  { label: "备份", href: "/settings/backups", icon: DatabaseBackup },
  { label: "设置", href: "/settings", icon: Settings },
];

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [session, setSession] = useState(readDevSession);

  useEffect(() => {
    const update = () => setSession(readDevSession());
    window.addEventListener(devSessionEvent, update);
    window.addEventListener("storage", update);
    update();
    return () => {
      window.removeEventListener(devSessionEvent, update);
      window.removeEventListener("storage", update);
    };
  }, []);

  useEffect(() => {
    if (!session.userId) {
      router.replace("/login");
    }
  }, [router, session.userId]);

  function logout() {
    apiDeleteSession();
    router.replace("/login");
  }

  if (!session.userId) {
    return <main className="min-h-dvh bg-[var(--surface-page)]" />;
  }

  return (
    <div className="min-h-dvh bg-[var(--surface-page)] text-[var(--text-primary)]">
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-[232px] border-r border-white/10 bg-[var(--surface-sidebar)] px-3 py-4 text-slate-300 lg:block">
        <SidebarContent pathname={pathname} role={session.role} />
      </aside>

      <div className="lg:pl-[232px]">
        <header className="sticky top-0 z-20 flex h-16 items-center justify-between border-b border-[var(--border-subtle)] bg-white/85 px-4 backdrop-blur md:px-6">
          <div className="flex items-center gap-3">
            <Button aria-label="打开导航" size="icon" type="button" variant="ghost" onClick={() => setMobileOpen(true)}>
              <Menu className="h-5 w-5" />
            </Button>
            <div className="hidden text-sm font-medium text-[var(--text-secondary)] sm:block">库存后台</div>
          </div>
          <div className="flex items-center gap-2">
            <div className="flex items-center gap-2 rounded-full border border-[var(--border-subtle)] bg-white px-3 py-1.5 text-xs text-[var(--text-secondary)]">
              <span className={cn("h-2 w-2 rounded-full", session.userId ? "bg-emerald-500" : "bg-amber-500")} />
              <span>{session.userId ? `${session.role} · ${session.userId.slice(0, 8)}` : "未设置身份"}</span>
            </div>
            <Button aria-label="退出登录" size="icon" title="退出登录" type="button" variant="ghost" onClick={logout}>
              <LogOut className="h-4 w-4" />
            </Button>
          </div>
        </header>

        <main className="mx-auto w-full max-w-[1440px] px-4 py-5 md:px-6 lg:py-6">{children}</main>
      </div>

      {mobileOpen ? (
        <div className="fixed inset-0 z-40 lg:hidden">
          <button className="absolute inset-0 bg-black/35" type="button" aria-label="关闭导航" onClick={() => setMobileOpen(false)} />
          <aside className="relative h-full w-[280px] bg-[var(--surface-sidebar)] px-3 py-4 text-slate-300 shadow-2xl">
            <div className="mb-3 flex justify-end">
              <Button aria-label="关闭导航" size="icon" type="button" variant="ghost" onClick={() => setMobileOpen(false)}>
                <X className="h-5 w-5 text-white" />
              </Button>
            </div>
            <SidebarContent pathname={pathname} role={session.role} onNavigate={() => setMobileOpen(false)} />
          </aside>
        </div>
      ) : null}
    </div>
  );
}

function SidebarContent({ pathname, role, onNavigate }: { pathname: string; role: Role; onNavigate?: () => void }) {
  return (
    <>
      <div className="mb-6 px-2">
        <div className="text-base font-semibold text-white">Gaowang</div>
        <div className="mt-1 text-xs text-slate-500">Inventory Command</div>
      </div>
      <nav className="grid gap-1">
        {navItems.filter((item) => !item.roles || item.roles.includes(role)).map((item) => {
          const active = pathname === item.href || (item.href !== "/dashboard" && pathname.startsWith(item.href));
          const Icon = item.icon;
          return (
            <Link
              className={cn(
                "flex h-9 items-center gap-2 rounded-md px-2.5 text-sm transition hover:bg-white/[0.07] hover:text-white",
                active ? "bg-white/10 text-white" : "text-slate-400",
              )}
              href={item.href}
              key={item.href}
              onClick={onNavigate}
            >
              <Icon className="h-4 w-4" />
              <span>{item.label}</span>
            </Link>
          );
        })}
      </nav>
    </>
  );
}
